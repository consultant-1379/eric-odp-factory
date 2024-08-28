package factory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"

	"eric-odp-factory/internal/common"
)

func (f *OdpFactoryImpl) handleConfigMapEventLoad(
	ctx context.Context,
	cmName string,
	configMap *corev1.ConfigMap,
) (*OdpTemplate, string) {
	application, applicationExists := configMap.Annotations[AnnApplication]
	if !applicationExists {
		slog.Error("handleConfigMapEvent Failed get application", common.CtxIDLabel, ctx.Value(common.CtxID),
			"ConfigMap", cmName)

		return nil, ""
	}

	accessGroupsStr, accessGroupsExists := configMap.Annotations[AnnAccessGroups]
	if !accessGroupsExists {
		slog.Error("handleConfigMapEvent Failed get accessgroups", common.CtxIDLabel, ctx.Value(common.CtxID),
			"ConfigMap", cmName)

		return nil, ""
	}

	odpTemplate := &OdpTemplate{
		ConfigMapName: cmName,
		Application:   application,
		AccessGroups:  strings.Split(accessGroupsStr, ","),
		Data:          make(map[string]string),
		PodTemplate:   nil,
	}

	for name, value := range configMap.Data {
		if name == "template" {
			podTemplate, err := parseOdpTemplate(application, value)
			if err == nil {
				odpTemplate.PodTemplate = podTemplate
			} else {
				slog.Error("handleConfigMapEvent Failed to parse template", "ConfigMap", cmName, "err", err)

				return nil, ""
			}
		} else {
			odpTemplate.Data[name] = value
		}
	}

	slog.Info("handleConfigMapEventLoad loaded ODP Template",
		common.CtxIDLabel, ctx.Value(common.CtxID),
		"ConfigMap", cmName, "Application", odpTemplate.Application)

	return odpTemplate, application
}

func (f *OdpFactoryImpl) handleConfigMapEvent(ctx context.Context, cmName string, obj interface{}) {
	startTime := time.Now()
	defer func() { recordK8sEventDuration("pod", time.Since(startTime).Seconds()) }()

	recordK8sEventInFlight("configmap", true)
	defer recordK8sEventInFlight("configmap", false)

	if obj == nil {
		slog.Info("handleConfigMapEvent deleting template", common.CtxIDLabel, ctx.Value(common.CtxID), "ConfigMap", cmName)

		odpTemplate, exists := f.templatesByName[cmName]
		if exists {
			delete(f.templatesByApplication, odpTemplate.Application)
			delete(f.templatesByName, cmName)
		}
	} else {
		odpTemplate, application := f.handleConfigMapEventLoad(ctx, cmName, obj.(*corev1.ConfigMap))
		if odpTemplate != nil {
			f.templatesByApplication[application] = odpTemplate
			f.templatesByName[cmName] = odpTemplate
		}
	}
}

func parseOdpTemplate(application, templateStr string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"odpReject": func(reason string) (string, error) {
			return "", &odpRejectionError{reason: reason}
		},
	}

	//nolint:wrapcheck // We'll call the error in the caller
	return template.New(application).Funcs(funcMap).Parse(templateStr)
}

type ConfigMapventHandler struct {
	odpFactoryImpl *OdpFactoryImpl
}

func (c *ConfigMapventHandler) HandleEvent(ctx context.Context, objectname string, objectdata interface{}) {
	c.odpFactoryImpl.handleConfigMapEvent(ctx, objectname, objectdata)
}

type odpRejectionError struct {
	reason string
}

func (e *odpRejectionError) Error() string {
	return fmt.Sprintf("ODP Template rejected request: %s", e.reason)
}
