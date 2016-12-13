package controller

import (
	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/api/v1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

type queue struct {
	addonch chan *spec.Addon
	spch    chan *spec.ServicePlan
	dpch    chan *extensions.Deployment
	nsch    chan *v1.Namespace
}

func newQueue(size int) *queue {
	return &queue{
		addonch: make(chan *spec.Addon, size),
		spch:    make(chan *spec.ServicePlan, size),
		dpch:    make(chan *extensions.Deployment),
		nsch:    make(chan *v1.Namespace, size),
	}
}

func (q *queue) add(o interface{}) {
	switch obj := o.(type) {
	case *spec.Addon:
		q.addonch <- o.(*spec.Addon)
	case *spec.ServicePlan:
		q.spch <- o.(*spec.ServicePlan)
	case *extensions.Deployment:
		q.dpch <- o.(*extensions.Deployment)
	case *v1.Namespace:
		q.nsch <- o.(*v1.Namespace)
	default:
		glog.Infof("add: unknown type (%T)", obj)
	}
}
func (q *queue) close() {
	close(q.addonch)
	close(q.spch)
	close(q.dpch)
	close(q.nsch)
}

func (q *queue) pop(o interface{}) (interface{}, bool) {
	switch t := o.(type) {
	case *spec.Addon:
		obj, ok := <-q.addonch
		return obj, ok
	case *spec.ServicePlan:
		obj, ok := <-q.spch
		return obj, ok
	case *extensions.Deployment:
		obj, ok := <-q.dpch
		return obj, ok
	case *v1.Namespace:
		obj, ok := <-q.nsch
		return obj, ok
	default:
		glog.Warningf("pop: unknown type (%T)", t)
		return nil, false
	}
}
