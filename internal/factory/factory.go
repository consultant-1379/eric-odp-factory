package factory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"

	"eric-odp-factory/internal/common"
	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/tokenservice"
	"eric-odp-factory/internal/userlookup"
)

const (
	Creating           = -1
	Ready              = 0
	LdapError          = 1
	TemplateError      = 2
	TokenError         = 3
	K8sError           = 4
	NotAuthorisedError = 5
	OtherError         = 9
)

const (
	AnnUserName     = "com.ericsson.odp.username"
	AnnApplication  = "com.ericsson.odp.application"
	AnnAccessGroups = "com.ericsson.odp.accessgroups"
	AnnPod          = "com.ericsson.odp.pod"
	AnnToken        = "com.ericsson.odp.token"     //nolint:gosec // Not an actual token
	AnnTokenType    = "com.ericsson.odp.tokentype" //nolint:gosec // Not an actual token

	LblToken    = "com.ericsson.odp.token" //nolint:gosec // Not an actual token
	LblPod      = "com.ericsson.odp.pod"
	LblTemplate = "com.ericsson.odp.template"
)

const (
	defaultErrorTTL             = 120
	defaultNumCreatePodRoutines = 5
	defaultCreatePodQueueSize   = 100
)

var (
	errNoTemplate    = errors.New("failed to get template for application")
	errOdpFailed     = errors.New("ODP failed")
	errOdpRejected   = errors.New("ODP Template rejected request")
	errNotAuthorized = errors.New("user not authorized for requested application")

	deletedCleanInterval = 60
	deletedTTL           = 300
)

type OnDemandPodParam struct {
	Username    string
	Application string
	RequestData map[string]string
	TokenTypes  []string
	InstanceID  string
	Profile     string
}

type OnDemandPod struct {
	// Username, Application, TokenType, Profile and RequestData
	// all come from the requests from the brokers
	Username    string
	Application string
	RequestData map[string]string
	TokenTypes  []string
	Profile     string

	// Generated Pod name
	PodName string

	// Data received from the ODP Token service
	TokenData map[string]string
	// Name of the of the ODP Token
	TokenName string

	// Indicates state of ODP Pod
	// -1 Creating
	//  0 Ready
	//  1 Failed looking up user in LDAP
	//  2 Failed generating On Demand Pod specification
	//  3 Failed to get requested ODP Token
	//  4 Failed to run ODP In Kubernetes
	//  5 Failed due to other reason
	ResultCode int

	// IP addresses of the Pod - only set when the Pod is running
	PodIPs []string

	// Error - only set if we fail to create the ODP
	// ErrorTime - time the error was set
	Error     error
	ErrorTime time.Time

	tRequested time.Time
	tCreating  time.Time
	tCreated   time.Time
	tReady     time.Time
}

type OdpTemplate struct {
	ConfigMapName string
	Application   string
	AccessGroups  []string
	Data          map[string]string
	PodTemplate   *template.Template
}

type OdpTemplateData struct {
	Application  string
	UserName     string
	TokenName    string
	TokenData    map[string]string
	ConfigData   map[string]string
	RequestData  map[string]string
	LdapUserAttr map[string]string
	LdapGroups   string
	PodName      string
	Profile      string
}

type OdpFactory interface {
	GetOnDemandPod(param *OnDemandPodParam) *OnDemandPod
	Stop()
}

type OdpFactoryImpl struct {
	clientset  kubernetes.Interface  // Kubernetes API
	userLookup userlookup.UserLookup // User Lookup (LDAP)

	odps          map[string]*OnDemandPod // ODPs by name
	odpsMutex     sync.RWMutex            // Protects odpsMutex
	createChannel chan string             // "Q" of ODPs to be created

	namespace string // Namespace we create the ODPs in
	errorTTL  int    // How log a failed ODP is kept
	odpPrefix string // Prefix for naming the ODPs

	templatesByApplication map[string]*OdpTemplate // ODP Templates by application
	templatesByName        map[string]*OdpTemplate // ODP Templates by ConfigMap name

	podwatcher *Controller // Watches for events from ODPs
	cmwatcher  *Controller // Watches for events from ODP Templates

	tsc tokenservice.Client // Token Service REST client

	// Recently deleted pods. We use this so we can ignore events from
	// ODPs that we get just after we've deleted the ODP
	deletedPods      map[types.UID]time.Time
	deletedPodsMutex sync.RWMutex // Protects deletedPods

	// Used when shutting down to wait for all go routines we started
	shutdownWG sync.WaitGroup
}

func NewOdpFactory(
	ctx context.Context,
	appConfig *config.Config,
	clientsetArg kubernetes.Interface,
	userLookupArg userlookup.UserLookup,
	tsc tokenservice.Client,
) *OdpFactoryImpl {
	slog.Info("StartFactory Entered")

	odpFactory := OdpFactoryImpl{
		namespace: appConfig.Namespace,
		errorTTL:  appConfig.FactoryErrorTime,
		odpPrefix: appConfig.FactoryOdpPrefix,

		clientset:              clientsetArg,
		userLookup:             userLookupArg,
		odps:                   make(map[string]*OnDemandPod),
		odpsMutex:              sync.RWMutex{},
		createChannel:          make(chan string, defaultCreatePodQueueSize),
		templatesByApplication: make(map[string]*OdpTemplate),
		templatesByName:        make(map[string]*OdpTemplate),
		tsc:                    tsc,

		deletedPods:      make(map[types.UID]time.Time),
		deletedPodsMutex: sync.RWMutex{},
		shutdownWG:       sync.WaitGroup{},
	}

	// This needs to be done before the Controllers are started
	// Otherwise we have a race condition where we could be
	// trying to record a handler being called before
	// the metrics have been created.
	setupMetrics(&odpFactory)

	odpFactory.podwatcher = StartController(
		ctx,
		odpFactory.clientset,
		odpFactory.namespace,
		&PodEventHandler{odpFactoryImpl: &odpFactory},
		"Pods",
		&corev1.Pod{},
		LblPod,
	)
	odpFactory.cmwatcher = StartController(
		ctx,
		odpFactory.clientset,
		odpFactory.namespace,
		&ConfigMapventHandler{odpFactoryImpl: &odpFactory},
		"ConfigMaps",
		&corev1.ConfigMap{},
		LblTemplate,
	)

	for i := 0; i < defaultNumCreatePodRoutines; i++ {
		createCtx := context.WithValue(ctx, common.CtxID, fmt.Sprintf("create-%d", i))
		odpFactory.shutdownWG.Add(1)
		go func(useCtx context.Context) {
			createOnDemandPods(useCtx, &odpFactory)
			odpFactory.shutdownWG.Done()
		}(createCtx)
	}

	odpFactory.shutdownWG.Add(1)
	go func() {
		odpFactory.deletedPodsCleaner(ctx)
		odpFactory.shutdownWG.Done()
	}()

	slog.Info("StartFactory complete")

	return &odpFactory
}

func (f *OdpFactoryImpl) Stop() {
	f.podwatcher.Stop()
	f.cmwatcher.Stop()
	f.shutdownWG.Wait()
}

func (f *OdpFactoryImpl) GetOnDemandPod(param *OnDemandPodParam) *OnDemandPod {
	slog.Info("GetOnDemandPod", "username", param.Username, "application", param.Application,
		"tokentypes", param.TokenTypes)

	podName := getPodName(f.odpPrefix, param.Username, param.Application, param.InstanceID)

	// check if we already have a queued request
	createRequired := false
	f.odpsMutex.Lock()
	odp, exists := f.odps[podName]
	if !exists || isOdpErrorExpired(odp, f.errorTTL) {
		odp = &OnDemandPod{
			Username:    param.Username,
			Application: param.Application,
			PodName:     podName,
			TokenTypes:  param.TokenTypes,
			Profile:     param.Profile,
			RequestData: param.RequestData,
			ResultCode:  Creating,
			tRequested:  time.Now(),
		}
		f.odps[podName] = odp
		createRequired = true
	}
	f.odpsMutex.Unlock()
	slog.Debug("GetOnDemandPod", "podName", podName, "exists", exists, "createRequired", createRequired)

	// If there is an error set then return it
	if !odp.ErrorTime.IsZero() {
		return odp
	}

	if createRequired {
		slog.Info("GetOnDemandPod sending create request", "podName", odp.PodName)
		f.createChannel <- odp.PodName
		recordCreateRequested()
	}

	slog.Debug("GetOnDemandPod returning")

	return odp
}

func (f *OdpFactoryImpl) createOnDemandPod(ctx context.Context, odp *OnDemandPod) {
	tStart := time.Now()
	slog.Info("createOnDemandPod", common.CtxIDLabel, ctx.Value(common.CtxID), "odp", odp)

	recordCreateInFlight(true)
	defer recordCreateInFlight(false)

	odp.tCreating = tStart
	if !odp.tRequested.IsZero() {
		recordOdpCreationLatency(odp.tCreating.Sub(odp.tRequested).Seconds())
	}

	odpTemplate, exists := f.templatesByApplication[odp.Application]
	if !exists {
		setOdpError(ctx, odp, fmt.Errorf("%w: %s", errNoTemplate, odp.Application), TemplateError)

		return
	}

	userInfo, err := f.userLookup.Lookup(ctx, odp.Username)
	tLookupDone := time.Now()
	recordCreateStage("userlookup", tLookupDone.Sub(tStart).Seconds())
	if err != nil {
		setOdpError(ctx, odp, err, LdapError)

		return
	}
	slog.Debug("createOnDemandPod", common.CtxIDLabel, ctx.Value(common.CtxID), "userInfo", userInfo)

	tGetPodSpecStart := tLookupDone

	if !f.isAccessAllowed(ctx, odpTemplate, userInfo) {
		setOdpError(ctx, odp, errNotAuthorized, NotAuthorisedError)

		return
	}

	// Need to set the Token now so it can be used by
	// the ODP Template
	if len(odp.TokenTypes) > 0 {
		err = f.createOdpToken(ctx, odp)
		tGetPodSpecStart = time.Now()
		recordCreateStage("createtoken", tGetPodSpecStart.Sub(tLookupDone).Seconds())

		if err != nil {
			return
		}
	}

	pod := f.getPodSpec(ctx, odpTemplate, odp, userInfo)
	tGetPodSpecEnd := time.Now()
	recordCreateStage("getpodspec", tGetPodSpecEnd.Sub(tGetPodSpecStart).Seconds())
	if pod == nil {
		return
	}

	pod.ObjectMeta.Name = odp.PodName
	pod.ObjectMeta.Namespace = f.namespace

	// Add odp label
	labels := pod.ObjectMeta.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[LblPod] = "true"
	pod.ObjectMeta.Labels = labels

	// Add username, appplication and token annotations
	annotations := pod.ObjectMeta.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[AnnUserName] = odp.Username
	annotations[AnnApplication] = odp.Application
	// Only if we have a token secret, add it to the annotations
	if odp.TokenName != "" {
		annotations[AnnToken] = odp.TokenName
	}
	pod.ObjectMeta.Annotations = annotations

	// Create the Pod
	err = createPod(ctx, f.clientset, f.namespace, odp.PodName, pod)
	tCreatePodEnd := time.Now()
	recordCreateStage("createpod", tCreatePodEnd.Sub(tGetPodSpecEnd).Seconds())

	odp.tCreated = time.Now()
	if err != nil {
		setOdpError(ctx, odp, err, K8sError)
		f.deleteOdpToken(ctx, odp)
	}

	tEnd := time.Now()
	recordCreateStage("e2e", tEnd.Sub(tStart).Seconds())
}

func (f *OdpFactoryImpl) isAccessAllowed(
	ctx context.Context,
	odpTemplate *OdpTemplate,
	userInfo *userlookup.UserInfo,
) bool {
	for _, allowedGroupName := range odpTemplate.AccessGroups {
		for _, memberOf := range userInfo.MemberOf {
			if memberOf == allowedGroupName {
				slog.Debug(
					"isAccessAllowed allowed",
					common.CtxIDLabel, ctx.Value(common.CtxID),
					"allowedGroupName", allowedGroupName,
					"memberOf", memberOf)

				return true
			}
		}
	}

	slog.Warn(
		"isAccessAllowed denied",
		common.CtxIDLabel, ctx.Value(common.CtxID),
		"user", userInfo.Username,
		"application", odpTemplate.Application,
		"accessGroups", odpTemplate.AccessGroups,
		"memberOf", userInfo.MemberOf)

	return false
}

func (f *OdpFactoryImpl) getPodSpec(
	ctx context.Context,
	odpTemplate *OdpTemplate,
	odp *OnDemandPod,
	userInfo *userlookup.UserInfo,
) *corev1.Pod {
	odpTempateData := OdpTemplateData{
		LdapUserAttr: userInfo.UserAttr,
		LdapGroups:   userInfo.Groups,
		Application:  odp.Application,
		UserName:     odp.Username,
		ConfigData:   odpTemplate.Data,
		RequestData:  odp.RequestData,
		TokenName:    odp.TokenName,
		TokenData:    odp.TokenData,
		PodName:      odp.PodName,
		Profile:      odp.Profile,
	}
	slog.Debug("getPodSpec", common.CtxIDLabel, ctx.Value(common.CtxID), "odpTempateData", odpTempateData)

	podSpecBuffer := new(bytes.Buffer)
	if err := odpTemplate.PodTemplate.Execute(podSpecBuffer, odpTempateData); err != nil {
		var rejectionErr *odpRejectionError
		if errors.As(err, &rejectionErr) {
			setOdpError(ctx, odp, fmt.Errorf("%w: %s", errOdpRejected, rejectionErr.reason), TemplateError)
		} else {
			setOdpError(ctx, odp, fmt.Errorf("failed to execute template: %w", err), TemplateError)
		}

		return nil
	}

	slog.Debug("getPodSpec", common.CtxIDLabel, ctx.Value(common.CtxID), "podSpecBuffer", podSpecBuffer)

	decoder := scheme.Codecs.UniversalDeserializer()
	podSpecObj, _, err := decoder.Decode(podSpecBuffer.Bytes(), nil, nil)
	if err != nil {
		setOdpError(ctx, odp, fmt.Errorf("failed to decode template %w", err), TemplateError)
		slog.Error("failed to deode", common.CtxIDLabel, ctx.Value(common.CtxID), "podSpecBuffer", podSpecBuffer)

		return nil
	}

	return podSpecObj.(*corev1.Pod)
}

func (f *OdpFactoryImpl) reloadOdp(ctx context.Context, podName string, pod *corev1.Pod, odp *OnDemandPod) {
	slog.Warn("reloadOdp", common.CtxIDLabel, ctx.Value(common.CtxID), "Pod", podName)
	for annotationName, annotationValue := range pod.GetAnnotations() {
		if annotationName == AnnUserName {
			odp.Username = annotationValue
		} else if annotationName == AnnApplication {
			odp.Application = annotationValue
		} else if annotationName == AnnToken {
			odp.TokenName = annotationValue
		}
	}

	if odp.TokenName != "" {
		if err := f.readOdpToken(ctx, odp); err != nil {
			slog.Error("failed to reload ODP Token", common.CtxIDLabel, ctx.Value(common.CtxID), "Pod", podName, "err", err)
		}
	}
}

func (f *OdpFactoryImpl) deletedPodsCleaner(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("deletedPodsCleaner shutting down")

			return

		case <-time.After(time.Second * time.Duration(deletedCleanInterval)):
			f.deletedPodsMutex.Lock()
			now := time.Now()
			for podid, deletedAt := range f.deletedPods {
				if now.Sub(deletedAt).Seconds() > float64(deletedTTL) {
					slog.Debug("deletedPodsCleaner removing", "podid", podid, "deletedAt", deletedAt)
					delete(f.deletedPods, podid)
				}
			}
			f.deletedPodsMutex.Unlock()
		}
	}
}

func createOnDemandPods(ctx context.Context, f *OdpFactoryImpl) {
	slog.Info("createOnDemandPods Started", common.CtxIDLabel, ctx.Value(common.CtxID))
	for {
		select {
		case <-ctx.Done():
			slog.Info("createOnDemandPods shutting down", common.CtxIDLabel, ctx.Value(common.CtxID))

			return

		case odpName := <-f.createChannel:
			f.odpsMutex.Lock()
			odp, exists := f.odps[odpName]
			f.odpsMutex.Unlock()
			slog.Debug("createOnDemandPods", common.CtxIDLabel, ctx.Value(common.CtxID), "odpName", odpName, "exists", exists)
			if exists {
				f.createOnDemandPod(ctx, odp)
			} else {
				slog.Warn("createOnDemandPods: Could not find odp", common.CtxIDLabel, ctx.Value(common.CtxID), "odpName", odpName)
			}
		}
	}
}
