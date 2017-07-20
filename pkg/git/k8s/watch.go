package k8s

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	resyncPeriod = 30 * time.Second
)

// PodWatcher is a struct which holds the return values of
// (k8s.io/kubernetes/pkg/controller/framework). NewIndexerInformer together.
type PodWatcher struct {
	Store      StoreToPodLister
	Controller *Controller
}

// StoreToPodLister makes a Store have the List method of the client.PodInterface
// The Store must contain (only) Pods.
//
// Example:
// s := cache.NewStore()
// lw := cache.ListWatch{Client: c, FieldSelector: sel, Resource: "pods"}
// r := cache.NewReflector(lw, &api.Pod{}, s).Run()
// l := StoreToPodLister{s}
// l.List()
type StoreToPodLister struct {
	cache.Indexer
}

// List | Please note that selector is filtering among the pods that have gotten into
// the store; there may have been some filtering that already happened before that.
// We explicitly don't return api.PodList, to avoid expensive allocations, which
// in most cases are unnecessary.
func (s *StoreToPodLister) List(selector labels.Selector) (pods []*v1.Pod, err error) {
	for _, m := range s.Indexer.List() {
		pod := m.(*v1.Pod)
		if selector.Matches(labels.Set(pod.Labels)) {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

// NewPodWatcher creates a new BuildPodWatcher useful to list the
// pods using a cache which gets updated based on the watch func.
func NewPodWatcher(c kubernetes.Interface, ns string) *PodWatcher {
	pw := &PodWatcher{}
	cacheStore, controller := NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  podListFunc(c, ns),
			WatchFunc: podWatchFunc(c, ns),
		},
		&v1.Pod{},
		resyncPeriod,
		ResourceEventHandlerFuncs{},
		cache.Indexers{},
	)
	pw.Store = StoreToPodLister{Indexer: cacheStore}
	pw.Controller = controller
	return pw
}

func podListFunc(c kubernetes.Interface, ns string) func(options metav1.ListOptions) (runtime.Object, error) {
	return func(opts metav1.ListOptions) (runtime.Object, error) {
		return c.Core().Pods(ns).List(metav1.ListOptions{})
	}
}

func podWatchFunc(c kubernetes.Interface, ns string) func(options metav1.ListOptions) (watch.Interface, error) {
	return func(opts metav1.ListOptions) (watch.Interface, error) {
		return c.Core().Pods(ns).Watch(metav1.ListOptions{})
	}
}
