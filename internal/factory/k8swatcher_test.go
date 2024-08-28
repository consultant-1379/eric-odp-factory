package factory

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

type CountingHandler struct {
	handleCount int
	t           *testing.T
}

func (h *CountingHandler) HandleEvent(_ context.Context, objectname string, _ interface{}) {
	h.handleCount++
	h.t.Logf("HandleEvent called objectname=%s", objectname)
}

func TestInvalidType(t *testing.T) {
	controller := StartController(
		context.TODO(),
		fake.NewSimpleClientset(),
		"testnamespace",
		nil,
		"Secrets",
		nil,
		"",
	)
	if controller != nil {
		t.Errorf("Expected nil controller: got %v", controller)
	}
}

func TestHandlerCalled(t *testing.T) {
	kubernetesClient := fake.NewSimpleClientset()

	watcherStarted := make(chan struct{})
	kubernetesClient.PrependWatchReactor(
		"*",
		func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
			gvr := action.GetResource()
			ns := action.GetNamespace()
			watcher, err := kubernetesClient.Tracker().Watch(gvr, ns)
			if err != nil {
				//nolint:wrapcheck // Suppress error returned from interface method should be wrapped
				return false, nil, err
			}
			close(watcherStarted)

			return true, watcher, nil
		},
	)

	handler := CountingHandler{t: t}

	ctx, cancelFunc := context.WithCancel(context.TODO())

	controller := StartController(
		ctx,
		kubernetesClient,
		"testnamespace",
		&handler,
		"Pods",
		&corev1.Pod{},
		LblPod,
	)

	<-watcherStarted
	t.Log("Watcher started")

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testapp-testuser",
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
	}

	kubernetesClient.CoreV1().Pods("testnamespace").Create(
		ctx,
		&pod,
		metav1.CreateOptions{},
	)

	for i := 0; i < 5 && handler.handleCount == 0; i++ {
		time.Sleep(1000 * time.Millisecond)
	}
	if handler.handleCount == 0 {
		t.Error("handler not called")
	}

	cancelFunc()
	controller.Stop()
}
