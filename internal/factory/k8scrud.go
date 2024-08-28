package factory

import (
	"context"
	"log/slog"
	"time"

	"eric-odp-factory/internal/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func recordMetrics(typename string, method string, start time.Time, err error) {
	duration := time.Since(start)
	recordK8sRequest(typename, method, duration.Seconds())
	if err != nil {
		recordK8sError(typename, method)
	}
}

func createPod(
	ctx context.Context,
	clientSet kubernetes.Interface,
	namespace string,
	name string, podSpec *corev1.Pod,
) error {
	slog.Info("K8S CRUD Create", common.CtxIDLabel, ctx.Value(common.CtxID), "Pod", name)
	start := time.Now()
	_, err := clientSet.CoreV1().Pods(namespace).Create(ctx, podSpec, metav1.CreateOptions{})
	recordMetrics("pod", "create", start, err)

	return err //nolint:wrapcheck // Wrapping done in factory.go
}

func deletePod(ctx context.Context, clientSet kubernetes.Interface, namespace string, name string) error {
	slog.Info("K8S CRUD Delete", common.CtxIDLabel, ctx.Value(common.CtxID), "Pod", name)
	start := time.Now()
	err := clientSet.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	recordMetrics("pod", "delete", start, err)

	return err //nolint:wrapcheck // Wrapping done in factory.go
}
