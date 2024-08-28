package factory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"eric-odp-factory/internal/common"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

func (f *OdpFactoryImpl) handlePodEvent(ctx context.Context, podName string, obj interface{}) {
	startTime := time.Now()
	defer func() { recordK8sEventDuration("pod", time.Since(startTime).Seconds()) }()
	recordK8sEventInFlight("pod", true)
	defer recordK8sEventInFlight("pod", false)

	slog.Debug("handlePodEvent", common.CtxIDLabel, ctx.Value(common.CtxID), "podName", podName, "obj is nil", obj == nil)

	f.odpsMutex.Lock()
	odp, odpExists := f.odps[podName]

	// Simplest case, Pod has been deleted, make sure we've removed from odps
	if obj == nil {
		// Note: if it's in a non-expired error state, then we'll keep it
		if odpExists && isOdpErrorExpired(odp, f.errorTTL) {
			delete(f.odps, podName)
		}
		f.odpsMutex.Unlock()

		return
	}

	pod := obj.(*corev1.Pod)

	// If we don't know about this ODP
	if !odpExists {
		// Check if we've just deleted it, if so ignore the event.
		f.deletedPodsMutex.Lock()
		_, deletedExists := f.deletedPods[pod.UID]
		f.deletedPodsMutex.Unlock()
		if deletedExists {
			slog.Info("handlePodEvent: dropping event for deleted Pod", common.CtxIDLabel, ctx.Value(common.CtxID),
				"PodName", podName)
			f.odpsMutex.Unlock()

			return
		}

		// Now we need to re-create the pod, normal use-case to get here is after a restart.
		odp = &OnDemandPod{
			PodName:    podName,
			ResultCode: Creating,
		}
		f.odps[podName] = odp
	}
	f.odpsMutex.Unlock()

	if !odpExists {
		f.reloadOdp(ctx, podName, pod, odp)
	}

	// Now check if the Pod is finsihed
	podPhase := pod.Status.Phase
	podFinished := false
	if podPhase == corev1.PodFailed || podPhase == corev1.PodSucceeded {
		podFinished = true
	}
	slog.Debug("handlePodEvent", common.CtxIDLabel, ctx.Value(common.CtxID),
		"podName", podName, "podPhase", podPhase, "podFinished", podFinished)

	if podFinished {
		f.handlePodFinished(ctx, podName, pod, odp)
	} else {
		f.handlePodUpdate(ctx, podName, pod, odp)
	}
}

func (f *OdpFactoryImpl) handlePodFinished(ctx context.Context, podName string, pod *corev1.Pod, odp *OnDemandPod) {
	podPhase := pod.Status.Phase

	if podPhase == corev1.PodFailed {
		slog.Error("handlePodEvent Pod failed", common.CtxIDLabel, ctx.Value(common.CtxID),
			"podName", podName, "podStatus", pod.Status)
		recordOdpFinished("failed")
		reason := getFailureReason(pod)
		if reason == "" {
			slog.Warn("Could not get failure reason", common.CtxIDLabel, ctx.Value(common.CtxID),
				"podName", podName, "pod.Status", pod.Status)
			reason = "Cause unknown"
		}

		setOdpError(ctx, odp, fmt.Errorf("%w: %s", errOdpFailed, reason), K8sError)
	} else if podPhase == corev1.PodSucceeded {
		recordOdpFinished("succeeded")
		// Don't need the odp any more so remove it
		f.odpsMutex.Lock()
		delete(f.odps, podName)
		f.odpsMutex.Unlock()
		// Remember that we've deleted this Pod is we can ignore any more events for this Pod.
		f.deletedPodsMutex.Lock()
		f.deletedPods[pod.UID] = time.Now()
		f.deletedPodsMutex.Unlock()
	}

	slog.Debug("handlePodFinished deleting ODP Token", common.CtxIDLabel, ctx.Value(common.CtxID),
		"podName", podName)
	if odp.TokenName != "" {
		f.deleteOdpToken(ctx, odp)
		odp.TokenName = ""
	}

	if err := deletePod(ctx, f.clientset, f.namespace, podName); err != nil {
		if !kerrors.IsNotFound(err) {
			slog.Error("handlePodFinished Failed to delete pod", "podName", podName, "err", err)
		}
	}
}

func (f *OdpFactoryImpl) handlePodUpdate(ctx context.Context, podName string, pod *corev1.Pod, odp *OnDemandPod) {
	if pod.Status.Phase == corev1.PodRunning {
		slog.Debug("handlePodUpdate update Odp to ready", common.CtxIDLabel, ctx.Value(common.CtxID), "podName", podName)
		odp.ResultCode = Ready
		odp.PodIPs = make([]string, 0, len(pod.Status.PodIPs))
		for _, podIP := range pod.Status.PodIPs {
			odp.PodIPs = append(odp.PodIPs, podIP.IP)
		}

		if odp.tReady.IsZero() && !odp.tCreated.IsZero() {
			odp.tReady = time.Now()
			recordOdpReadyLatency(odp.Application, odp.tReady.Sub(odp.tCreated).Seconds())
			if !odp.tCreating.IsZero() {
				recordOdpEeELatency(odp.Application, odp.tReady.Sub(odp.tCreating).Seconds())
			}
		}
	}
}

type PodEventHandler struct {
	odpFactoryImpl *OdpFactoryImpl
}

func (p *PodEventHandler) HandleEvent(ctx context.Context, objectname string, objectdata interface{}) {
	p.odpFactoryImpl.handlePodEvent(ctx, objectname, objectdata)
}
