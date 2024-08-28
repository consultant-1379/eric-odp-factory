package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"eric-odp-factory/internal/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	numRetries          = 5
	numPodWorkers       = 5
	numConfigMapWorkers = 1
)

type Controller struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller

	namespace    string
	stop         chan struct{}
	waitGroup    *sync.WaitGroup
	eventHandler EventHandler
	typename     string
}

type eventKeyType struct {
	key       string
	eventType string
}

type EventHandler interface {
	HandleEvent(ctx context.Context, name string, objectdata interface{})
}

var errCacheWaitTimeout = errors.New("timed out waiting for caches to sync")

func StartController(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	handler EventHandler,
	typename string,
	objType runtime.Object,
	label string,
) *Controller {
	odpSelector := labels.SelectorFromSet(labels.Set(map[string]string{label: "true"})).String()

	var numWorkers int
	var listWatcher cache.ListerWatcher
	if typename == "Pods" {
		listWatcher = podListerWatcher{Interface: clientset, namespace: namespace, selector: odpSelector}
		numWorkers = numPodWorkers
	} else if typename == "ConfigMaps" {
		listWatcher = configMapListerWatcher{Interface: clientset, namespace: namespace, selector: odpSelector}
		numWorkers = numConfigMapWorkers
	} else {
		slog.Error("Unknown type", "typename", typename)

		return nil
	}

	// create the workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Bind the workqueue to a cache with the help of an informer. This way we make sure that
	// whenever the cache is updated, the pod key is added to the workqueue.
	// Note that when we finally process the item from the workqueue, we might see a newer version
	// of the Pod than the version which was responsible for triggering the update.
	var event eventKeyType
	var err error
	indexer, informer := cache.NewIndexerInformer(
		listWatcher,
		objType,
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				event.key, err = cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					event.eventType = "create"
					queue.Add(event)
				}
			},
			UpdateFunc: func(_ interface{}, new interface{}) {
				event.key, err = cache.MetaNamespaceKeyFunc(new)
				if err == nil {
					event.eventType = "update"
					queue.Add(event)
				}
			},
			DeleteFunc: func(obj interface{}) {
				// IndexerInformer uses a delta queue, therefore for deletes we have to use this
				// key function.
				event.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					event.eventType = "delete"
					queue.Add(event)
				}
			},
		},
		cache.Indexers{},
	)

	controller := &Controller{
		informer: informer,
		indexer:  indexer,
		queue:    queue,

		stop:         make(chan struct{}),
		namespace:    namespace,
		eventHandler: handler,
		typename:     typename,
	}

	started := sync.WaitGroup{}
	started.Add(1)

	ctrlCtx := context.WithValue(ctx, common.CtxID, fmt.Sprintf("ctrl-run-%s", typename))
	// Now let's start the controller
	go controller.Run(ctrlCtx, numWorkers, &started, controller.stop)

	started.Wait()

	return controller
}

func (c *Controller) Stop() {
	slog.Info("watcher stopping", "typename", c.typename)
	close(c.stop)
	c.waitGroup.Wait()
	slog.Info("watcher stopped", "typename", c.typename)
}

// Run begins watching and syncing.
func (c *Controller) Run(ctx context.Context, numWorkers int, started *sync.WaitGroup, stopCh chan struct{}) {
	defer utilruntime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	slog.Info("watcher starting", common.CtxIDLabel, ctx.Value(common.CtxID), "typename", c.typename)

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(errCacheWaitTimeout)

		return
	}

	c.waitGroup = &sync.WaitGroup{}
	for i := 0; i < numWorkers; i++ {
		workerCtx := context.WithValue(ctx, common.CtxID, fmt.Sprintf("ctrl-wrkr-%s-%d", c.typename, i))
		c.waitGroup.Add(1)
		go func(useCtx context.Context) {
			defer c.waitGroup.Done()
			for c.processNextItem(useCtx) {
			}
		}(workerCtx)
	}

	started.Done()

	<-stopCh
}

func (c *Controller) handleItem(ctx context.Context, e eventKeyType) error {
	slog.Debug("handleItem", common.CtxIDLabel, ctx.Value(common.CtxID), "typename", c.typename)

	obj, _, err := c.indexer.GetByKey(e.key)
	if err != nil {
		wrappedErr := fmt.Errorf("fetching object with key %s from store failed with %w", e.key, err)
		slog.Error("handleItem failed", common.CtxIDLabel, ctx.Value(common.CtxID), "wrappedErr", wrappedErr)

		return wrappedErr
	}

	// Strip off the namespace prefix
	localName := e.key[len(c.namespace)+1:]
	c.eventHandler.HandleEvent(ctx, localName, obj)

	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(ctx context.Context, err error, e interface{}) {
	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(e) < numRetries {
		slog.Warn("Error syncing pod", common.CtxIDLabel, ctx.Value(common.CtxID), "e", e, "err", err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(e)

		return
	}

	c.queue.Forget(e)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	utilruntime.HandleError(err)
	slog.Warn("Dropping pod out of the queue", common.CtxIDLabel, ctx.Value(common.CtxID), "pod", e, "err", err)
}

func (c *Controller) processNextItem(ctx context.Context) bool {
	e, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(e)

	if err := c.handleItem(ctx, e.(eventKeyType)); err != nil {
		c.handleErr(ctx, err, e)
	} else {
		c.queue.Forget(e)
	}

	return true
}

type podListerWatcher struct {
	kubernetes.Interface
	namespace string
	selector  string
}

func (p podListerWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	options.LabelSelector = p.selector

	//nolint:wrapcheck // Suppress error returned from interface method should be wrapped
	return p.Interface.CoreV1().Pods(p.namespace).List(context.Background(), options)
}

func (p podListerWatcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	options.LabelSelector = p.selector

	//nolint:wrapcheck // Suppress error returned from interface method should be wrapped
	return p.Interface.CoreV1().Pods(p.namespace).Watch(context.Background(), options)
}

type configMapListerWatcher struct {
	kubernetes.Interface
	namespace string
	selector  string
}

func (p configMapListerWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	options.LabelSelector = p.selector

	//nolint:wrapcheck // Suppress error returned from interface method should be wrapped
	return p.Interface.CoreV1().ConfigMaps(p.namespace).List(context.Background(), options)
}

func (p configMapListerWatcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	options.LabelSelector = p.selector

	//nolint:wrapcheck // Suppress error returned from interface method should be wrapped
	return p.Interface.CoreV1().ConfigMaps(p.namespace).Watch(context.Background(), options)
}
