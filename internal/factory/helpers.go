package factory

import (
	"context"
	"crypto/md5" //nolint:gosec // Not being used for crypto
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"eric-odp-factory/internal/common"

	corev1 "k8s.io/api/core/v1"
)

func getPodName(prefix string, username string, application string, instanceid string) string {
	hash := md5.New()              //nolint:gosec // Not being used for crypto purposes
	io.WriteString(hash, username) //nolint:errcheck // Should never fail
	if instanceid != "" {
		io.WriteString(hash, instanceid) //nolint:errcheck // Should never fail
	}

	return fmt.Sprintf("%s-%s-%x", prefix, application, hash.Sum(nil))
}

func setOdpError(ctx context.Context, odp *OnDemandPod, err error, errCode int) {
	slog.Warn("setOdpError", common.CtxIDLabel, ctx.Value(common.CtxID),
		"Pod", odp.PodName, "err", err)
	odp.ErrorTime = time.Now()
	odp.Error = err
	odp.ResultCode = errCode
}

func isOdpErrorExpired(odp *OnDemandPod, errorTTL int) bool {
	result := false

	slog.Debug("isOdpErrorExpired", "Pod", odp.PodName, "ErrorTime", odp.ErrorTime)
	if !odp.ErrorTime.IsZero() {
		secSinceErrorSet := time.Since(odp.ErrorTime).Seconds()
		slog.Debug("isOdpErrorExpired", "Pod", odp.PodName, "secSinceErrorSet", secSinceErrorSet)
		if secSinceErrorSet > float64(errorTTL) {
			result = true
		}
	}
	slog.Debug("isOdpErrorExpired", "Pod", odp.PodName, "result", result)

	return result
}

func getFailureReason(pod *corev1.Pod) string {
	reasons := make([]string, 0)

	if pod.Status.Message != "" {
		reasons = append(reasons, "Message: "+pod.Status.Message)
	}

	if pod.Status.Reason != "" {
		reasons = append(reasons, "Reason: "+pod.Status.Reason)
	}

	reasons = getFailureReasonFromContainers(pod.Status.InitContainerStatuses, "InitContainer", reasons)
	reasons = getFailureReasonFromContainers(pod.Status.ContainerStatuses, "Container", reasons)
	reasons = getFailureReasonFromContainers(pod.Status.EphemeralContainerStatuses, "EphemeralContainer", reasons)

	return strings.Join(reasons, ", ")
}

func getFailureReasonFromContainers(csl []corev1.ContainerStatus, containerType string, reasons []string) []string {
	for index := range csl {
		if csl[index].State.Terminated != nil && csl[index].State.Terminated.ExitCode != 0 {
			reasons = append(
				reasons,
				fmt.Sprintf(
					"%s %s terminated with exit code %d and message %s",
					containerType,
					csl[index].Name,
					csl[index].State.Terminated.ExitCode,
					csl[index].State.Terminated.Message,
				),
			)
		}
	}

	return reasons
}
