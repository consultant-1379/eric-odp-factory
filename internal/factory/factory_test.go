package factory

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/tokenservice"
	"eric-odp-factory/internal/userlookup"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/kubernetes/fake"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	k8stesting "k8s.io/client-go/testing"
)

func injectAppConfigMap(ctx context.Context, odpFactoryImpl *OdpFactoryImpl) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testconfigmap",
			Namespace: "testnamespace",
			Labels:    map[string]string{"com.ericsson.odp.template": "true"},
			Annotations: map[string]string{
				"com.ericsson.odp.application":  "testapp",
				"com.ericsson.odp.accessgroups": "grp1,grp2",
			},
		},
		Data: map[string]string{
			"template": `apiVersion: v1
kind: Pod
metadata:
name: odp-template-testapp
spec:
containers:
- name: testcon
  image: registry.access.redhat.com/ubi8/ubi:8.9
  command: ["sleep",  "30" ]`,
		},
	}

	odpFactoryImpl.handleConfigMapEvent(ctx, "testconfigmap", cm)
}

var errTest = errors.New("test error")

type TestTokenServiceClient struct {
	odpToken tokenservice.OdpToken
	err      error

	creates int
	gets    int
	deletes int
}

func (ttsc *TestTokenServiceClient) CreateToken(
	_ context.Context,
	_ string,
	_ []string,
) (*tokenservice.OdpToken, error) {
	ttsc.creates++

	return &ttsc.odpToken, ttsc.err
}

func (ttsc *TestTokenServiceClient) GetToken(_ context.Context, _ string) (*tokenservice.OdpToken, error) {
	ttsc.gets++

	return &ttsc.odpToken, ttsc.err
}

func (ttsc *TestTokenServiceClient) DeleteToken(_ context.Context, _ string) error {
	ttsc.deletes++

	return ttsc.err
}

var okayTTSC = TestTokenServiceClient{
	odpToken: tokenservice.OdpToken{
		Name: "testapp-testuser-odptoken",
		Data: map[string]string{"tokentype": "testtokentype"},
	},
}

func TestGetOnDemandPodOkay(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	// Let the watchers finish starting
	time.Sleep(100 * time.Millisecond)

	// Create ODP Template and wait until it's available
	injectAppConfigMap(ctx, odpFactoryImpl)

	keepgoing := true
	loopCount := 0
	podClient := kubernetesClient.CoreV1().Pods("testnamespace")
	var odp *OnDemandPod
	odpParam := OnDemandPodParam{
		Username:    "testuser",
		Application: "testapp",
		TokenTypes:  []string{"testtokentype"},
	}
	for keepgoing && loopCount < 10 {
		loopCount++
		odp = odpFactoryImpl.GetOnDemandPod(&odpParam)
		keepgoing = (odp.ResultCode == Creating)
		t.Logf("odp: %v", odp)

		// Make sure Pod gets into Running state
		pod, err := podClient.Get(context.Background(), odp.PodName, metav1.GetOptions{})
		if err == nil {
			if pod.Status.Phase != corev1.PodRunning {
				t.Log("Updating pod.Status.Phase")
				pod.Status.Phase = corev1.PodRunning
				podClient.Update(context.Background(), pod, metav1.UpdateOptions{})
			}
		}

		if keepgoing {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if keepgoing || !odp.ErrorTime.IsZero() {
		t.Errorf("TestGetOnDemandPodOkay: pod not ready %v", odp)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestGetOnDemandPodFailLDAP(t *testing.T) {
	appConfig := config.Config{Namespace: "testnamespace", FactoryErrorTime: 120, FactoryOdpPrefix: "eric-odp"}

	kubernetesClient := fake.NewSimpleClientset()

	userLookup := userlookup.Test{User: userlookup.UserInfo{}, Err: errTest}
	ctx, cancelFunc := context.WithCancel(context.TODO())
	odpFactoryImpl := NewOdpFactory(ctx, &appConfig, kubernetesClient, &userLookup, &okayTTSC)

	// Let the watchers finish starting
	time.Sleep(100 * time.Millisecond)

	// Create ODP Template and wait until it's available
	injectAppConfigMap(ctx, odpFactoryImpl)

	odp := createAndWait(odpFactoryImpl, t)
	if odp != nil && odp.ResultCode != LdapError {
		t.Errorf("TestGetOnDemandPodFailLDAP: pod not in correct failed state %v", odp)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestGetOnDemandPodFailNotAuthorised(t *testing.T) {
	appConfig := config.Config{Namespace: "testnamespace", FactoryErrorTime: 120, FactoryOdpPrefix: "eric-odp"}

	kubernetesClient := fake.NewSimpleClientset()

	userLookup := userlookup.Test{User: userlookup.UserInfo{Username: "testuser", MemberOf: []string{"someothergroup"}}}
	ctx, cancelFunc := context.WithCancel(context.TODO())
	odpFactoryImpl := NewOdpFactory(ctx, &appConfig, kubernetesClient, &userLookup, &okayTTSC)

	// Let the watchers finish starting
	time.Sleep(100 * time.Millisecond)

	// Create ODP Template and wait until it's available
	injectAppConfigMap(ctx, odpFactoryImpl)

	odp := createAndWait(odpFactoryImpl, t)
	if odp != nil && odp.ResultCode != NotAuthorisedError {
		t.Errorf("TestGetOnDemandPodFailNotAuthorised: pod not in correct failed state %v", odp)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestGetOnDemandPodFailTemplate(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, _, cancelFunc := setupFactory(kubernetesClient)

	// Let the watchers finish starting
	time.Sleep(100 * time.Millisecond)

	odp := createAndWait(odpFactoryImpl, t)
	if odp != nil && odp.ResultCode != TemplateError {
		t.Errorf("TestGetOnDemandPodFailTemplate: pod not in correct failed state %v", odp)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestGetOnDemandPodFailK8SPod(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	fakeClientCoreV1 := kubernetesClient.CoreV1().(*fakecorev1.FakeCoreV1)
	fakeClientCoreV1.PrependReactor(
		"create",
		"pods",
		func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, &corev1.Pod{}, errTest
		},
	)

	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	// Let the watchers finish starting
	time.Sleep(100 * time.Millisecond)

	// Create ODP Template and wait until it's available
	injectAppConfigMap(ctx, odpFactoryImpl)

	odp := createAndWait(odpFactoryImpl, t)
	if odp != nil && odp.ResultCode != K8sError {
		t.Errorf("TestGetOnDemandPodFailK8S: pod not in correct failed state %v", odp)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestHandleConfigMapEventDelete(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()
	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	injectAppConfigMap(ctx, odpFactoryImpl)

	// Now delete the ConfigMap
	odpFactoryImpl.handleConfigMapEvent(ctx, "testconfigmap", nil)

	// Now verify configmap has been removed
	if _, exists := odpFactoryImpl.templatesByName["testconfigmap"]; exists {
		t.Error("TestHandleConfigMapEventDelete: expected testconfigmap to have been removed from templatesByName")
	}
	if _, exists := odpFactoryImpl.templatesByApplication["testapp"]; exists {
		t.Error("TestHandleConfigMapEventDelete: expected testapp to have been removed from templatesByApplication")
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestHandleConfigMapEventLoadMissingApp(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	odpTemplate, _ := odpFactoryImpl.handleConfigMapEventLoad(ctx, "test", &corev1.ConfigMap{})
	if odpTemplate != nil {
		t.Error("TestHandleConfigMapEventLoadMissingApp: ex")
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestHandleConfigMapEventInvalidTemplate(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{AnnApplication: "testapp"},
		},
		Data: map[string]string{"aname": "avalue", "template": "{{ index "},
	}
	odpTemplate, _ := odpFactoryImpl.handleConfigMapEventLoad(ctx, "test", &configMap)
	if odpTemplate != nil {
		t.Error("expected error")
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestHandlePodUpdateSuccess(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	pod := makePodSpec()

	// Create Pod in K8S
	pod, _ = kubernetesClient.CoreV1().Pods("testnamespace").Create(
		context.Background(),
		pod,
		metav1.CreateOptions{},
	)

	expectedPodName := pod.Name

	// Wait for factory to pickup the created Pod
	odpExists := false
	for odpExists == false {
		_, odpExists = odpFactoryImpl.odps[expectedPodName]
		if !odpExists {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Now update the Pod to a finished state
	pod.Status.Phase = corev1.PodSucceeded
	pod, _ = kubernetesClient.CoreV1().Pods("testnamespace").Update(
		context.Background(),
		pod,
		metav1.UpdateOptions{},
	)

	// Now wait for the ODP to be removed
	for odpExists {
		_, odpExists = odpFactoryImpl.odps[expectedPodName]
		if odpExists {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Make sure we've deleted the pod
	_, getErr := kubernetesClient.CoreV1().Pods("testnamespace").Get(
		context.Background(),
		expectedPodName,
		metav1.GetOptions{},
	)
	if !k8serrors.IsNotFound(getErr) {
		t.Errorf("TestHandlePodUpdateSuccess: Expected Pod IsNotFound, got %v", getErr)
	}

	if _, inDeleted := odpFactoryImpl.deletedPods[pod.UID]; !inDeleted {
		t.Errorf("Expected to find pod in deletedPods: %v", odpFactoryImpl.deletedPods)
	}

	// Now send another event for the Pod
	expectedGets := okayTTSC.gets
	odpFactoryImpl.handlePodEvent(ctx, expectedPodName, pod)
	if okayTTSC.gets != expectedGets {
		t.Errorf("Expected no tokenservice.GetToken calls after event for deleted Pod: %v", odpFactoryImpl.deletedPods)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestHandlePodUpdateFailed(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, _, cancelFunc := setupFactory(kubernetesClient)

	pod := makePodSpec()

	// Create Pod in K8S
	kubernetesClient.CoreV1().Pods("testnamespace").Create(
		context.Background(),
		pod,
		metav1.CreateOptions{},
	)

	expectedOdpName := pod.Name
	// Wait for factory to pickup the created Pod
	odpExists := false
	for odpExists == false {
		_, odpExists = odpFactoryImpl.odps[expectedOdpName]
		if !odpExists {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Now update the Pod to a finished state
	pod.Status.Phase = corev1.PodFailed
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name: "testcontainer",
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 1,
					Message:  "Container failed testreason",
				},
			},
		},
	}

	kubernetesClient.CoreV1().Pods("testnamespace").Update(
		context.Background(),
		pod,
		metav1.UpdateOptions{},
	)

	// Now wait for the ODP to be updated to an error state
	isOdpErrored := false
	var odp *OnDemandPod
	odpParam := OnDemandPodParam{
		Username:    "testuser",
		Application: "testapp",
		TokenTypes:  []string{"testtokentype"},
	}
	for !isOdpErrored {
		odp = odpFactoryImpl.GetOnDemandPod(&odpParam)

		isOdpErrored = !odp.ErrorTime.IsZero()
		if !isOdpErrored {
			time.Sleep(100 * time.Millisecond)
		}
	}

	expectedError := "ODP failed: Container testcontainer terminated with exit code 1 and message Container failed testreason" //nolint:lll // Ignore
	if odp.Error.Error() != expectedError {
		t.Errorf("TestHandlePodUpdateFailed: unexpected error in ODP expected\n%s\ngot\n%v", expectedError, odp.Error)
	}

	// Make sure we've deleted the pod
	_, getErr := kubernetesClient.CoreV1().Pods("testnamespace").Get(
		context.Background(),
		expectedOdpName,
		metav1.GetOptions{},
	)
	if !k8serrors.IsNotFound(getErr) {
		t.Errorf("TestHandlePodUpdateSuccess: Expected Pod IsNotFound, got %v", getErr)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestHandlePodUpdateUnknown(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				AnnUserName:    "testuser",
				AnnApplication: "testapp",
				AnnToken:       "testapp-testuser-odptoken",
			},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			PodIPs: []corev1.PodIP{{IP: "1.2.3.4"}},
		},
	}

	odpFactoryImpl.handlePodEvent(ctx, "testapp-testuser", &pod)
	odp, odpExists := odpFactoryImpl.odps["testapp-testuser"]
	if odpExists {
		odpExpected := OnDemandPod{
			Username:    "testuser",
			Application: "testapp",
			TokenData:   map[string]string{"tokentype": "testtokentype"},
			PodName:     "testapp-testuser",
			TokenName:   "testapp-testuser-odptoken",
			PodIPs:      []string{"1.2.3.4"},
		}
		if !reflect.DeepEqual(*odp, odpExpected) {
			t.Errorf("TestHandlePodUpdateUnknown: expected %v got %v", odpExpected, *odp)
		}
	} else {
		t.Error("TestHandlePodUpdateUnknown: testapp-testuser not found")
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestOdpReject(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()
	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	// Validate that getPodSpec fails when the execute template
	// is calls odpReject
	templateStr := "{{odpReject \"Rejection Reason\"}}"
	parsedTemplate, _ := parseOdpTemplate("testapp", templateStr)
	odpTemplate := OdpTemplate{PodTemplate: parsedTemplate}
	odpFactoryImpl.templatesByApplication["testapp"] = &odpTemplate

	userInfo := userlookup.UserInfo{}
	odp := OnDemandPod{Application: "testapp"}
	podSpec := odpFactoryImpl.getPodSpec(ctx, &odpTemplate, &odp, &userInfo)

	if podSpec != nil {
		t.Error("expect nil podSpec")
	}

	if odp.ResultCode != TemplateError {
		t.Errorf("expected ErrorCode %d, got %v", TemplateError, odp)
	}

	expectedErrMsg := "ODP Template rejected request: Rejection Reason"
	if odp.Error.Error() != expectedErrMsg {
		t.Errorf("Expected err msg = \"%s\" got \"%s\"", expectedErrMsg, odp.Error.Error())
	}
	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestProfile(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()
	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	// Validate that getPodSpec fails when the execute template
	// is calls odpReject
	templateStr := `
{{- if ne .Profile "testprofile" -}}
{{odpReject "Wrong Profile"}}
{{- end -}}
apiVersion: v1
kind: Pod
metadata:
name: odp-template-testapp
spec:
containers:
- name: testcon
  image: registry.access.redhat.com/ubi8/ubi:8.9
  command: ["sleep",  "30" ]`

	parsedTemplate, parseErr := parseOdpTemplate("testapp", templateStr)
	if parseErr != nil {
		t.Fatalf("Failed to parse template: %v", parseErr)
	}
	odpTemplate := OdpTemplate{PodTemplate: parsedTemplate}
	odpFactoryImpl.templatesByApplication["testapp"] = &odpTemplate

	userInfo := userlookup.UserInfo{}
	odp := OnDemandPod{Application: "testapp", Profile: "wrongprofile"}
	podSpec := odpFactoryImpl.getPodSpec(ctx, &odpTemplate, &odp, &userInfo)

	if podSpec != nil {
		t.Error("expect nil podSpec")
	}

	expectedErrMsg := "ODP Template rejected request: Wrong Profile"
	if odp.Error.Error() != expectedErrMsg {
		t.Errorf("Expected err msg = \"%s\" got \"%s\"", expectedErrMsg, odp.Error.Error())
	}

	if odp.ResultCode != TemplateError {
		t.Errorf("expected ErrorCode %d, got %v", TemplateError, odp)
	}

	odp = OnDemandPod{Application: "testapp", Profile: "testprofile"}
	podSpec = odpFactoryImpl.getPodSpec(ctx, &odpTemplate, &odp, &userInfo)

	if podSpec == nil {
		t.Error("expect podSpec")
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestGetPodSpec(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()
	odpFactoryImpl, ctx, cancelFunc := setupFactory(kubernetesClient)

	// Validate that getPodSpec fails when we have a template
	// execute failure. Here, the template contains a reference
	// to an non-existent key.
	templateStr := "{{ index .InvalidRef \"invalidKey\" }}"
	parsedTemplate, _ := parseOdpTemplate("testapp", templateStr)
	odpTemplate := OdpTemplate{PodTemplate: parsedTemplate}
	odpFactoryImpl.templatesByApplication["testapp"] = &odpTemplate

	userInfo := userlookup.UserInfo{}
	odp := OnDemandPod{Application: "testapp"}
	podSpec := odpFactoryImpl.getPodSpec(ctx, &odpTemplate, &odp, &userInfo)

	if podSpec != nil {
		t.Error("expect nil podSpec")
	}

	if odp.ResultCode != TemplateError {
		t.Errorf("expected ErrorCode %d, got %v", TemplateError, odp)
	}

	// Validate that getPodSpec fails when the execute template
	// is not a valid PodSpec
	templateStr = "{{ .Application }}"
	parsedTemplate, _ = parseOdpTemplate("testapp", templateStr)
	odpTemplate = OdpTemplate{PodTemplate: parsedTemplate}
	odpFactoryImpl.templatesByApplication["testapp"] = &odpTemplate

	odp = OnDemandPod{Application: "testapp"}
	podSpec = odpFactoryImpl.getPodSpec(ctx, &odpTemplate, &odp, &userInfo)

	if podSpec != nil {
		t.Error("expect nil podSpec")
	}

	if odp.ResultCode != TemplateError {
		t.Errorf("expected ErrorCode %d, got %v", TemplateError, odp)
	}

	cancelFunc()
	odpFactoryImpl.Stop()
}

func TestDeletePodsCleaner(t *testing.T) {
	holdDeletedCleanInterval := deletedCleanInterval
	deletedCleanInterval = 1

	odpFactoryImpl := OdpFactoryImpl{
		deletedPods:      make(map[types.UID]time.Time),
		deletedPodsMutex: sync.RWMutex{},
	}
	now := time.Now()
	odpFactoryImpl.deletedPods["uid1"] = now.Add(-600 * time.Second)
	odpFactoryImpl.deletedPods["uid2"] = now.Add(-1 * time.Second)

	ctx, cancelFunc := context.WithCancel(context.TODO())

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		odpFactoryImpl.deletedPodsCleaner(ctx)
		wg.Done()
	}()

	// Wait for 2 seconds to make sure that the cleaner has run.
	time.Sleep(time.Second * 2)

	cancelFunc()

	wg.Wait()

	if _, exists := odpFactoryImpl.deletedPods["uid1"]; exists {
		t.Error("Expected uid1 to have been removed")
	}

	if _, exists := odpFactoryImpl.deletedPods["uid2"]; !exists {
		t.Error("Expected uid2 to have been left")
	}

	deletedCleanInterval = holdDeletedCleanInterval
}

func setupFactory(kubernetesClient *fake.Clientset) (*OdpFactoryImpl, context.Context, context.CancelFunc) {
	userInfo := userlookup.UserInfo{
		Username: "testuser",
		UserAttr: map[string]string{"name1": "value1"},
		Groups:   "grp1:1@grp2:2",
		MemberOf: []string{"grp1", "grp2"},
	}
	userLookup := userlookup.Test{User: userInfo, Err: nil}

	ctx, cancelFunc := context.WithCancel(context.TODO())
	appConfig := config.Config{Namespace: "testnamespace", FactoryErrorTime: 120, FactoryOdpPrefix: "eric-odp"}

	return NewOdpFactory(ctx, &appConfig, kubernetesClient, &userLookup, &okayTTSC), ctx, cancelFunc
}

func makePodSpec() *corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eric-odp-testapp-5d9c68c6c50ed3d02a2fcf54f63993b6",
			Namespace: "testnamespace",
			Labels: map[string]string{
				LblPod: "true",
			},
			Annotations: map[string]string{
				AnnUserName:    "testuser",
				AnnApplication: "testapp",
				AnnToken:       "testapp-testuser-odptoken",
			},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			PodIPs: []corev1.PodIP{{IP: "1.2.3.4"}},
		},
	}

	return &pod
}

func createAndWait(odpFactoryImpl *OdpFactoryImpl, t *testing.T) *OnDemandPod {
	keepgoing := true
	loopCount := 0
	var odp *OnDemandPod
	odpParam := OnDemandPodParam{
		Username:    "testuser",
		Application: "testapp",
		TokenTypes:  []string{"testtokentype"},
	}
	for keepgoing && loopCount < 10 {
		loopCount++
		odp = odpFactoryImpl.GetOnDemandPod(&odpParam)
		t.Logf("odp: %v", odp)

		keepgoing = (odp.ResultCode == Creating)
		if keepgoing {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if odp == nil {
		t.Error("Failed to create ODP")
	}

	return odp
}
