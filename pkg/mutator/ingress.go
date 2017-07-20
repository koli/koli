package mutator

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/apis/v1alpha1/draft"
	"kolihub.io/koli/pkg/util"
)

func (h *Handler) IngressOnCreate(w http.ResponseWriter, r *http.Request) {
	namespace := mux.Vars(r)["namespace"]
	key := fmt.Sprintf("Req-ID=%s, Resource=ingress:%s", r.Header.Get("X-Request-ID"), namespace)
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)
	new := draft.NewIngress(&v1beta1.Ingress{})

	if err := util.NewDecoder(r.Body, extensionsCodec).Decode(new); err != nil {
		msg := fmt.Sprintf("failed decoding request body [%v]", err)
		glog.V(3).Infof("%s -  %s", key, msg)
		util.WriteResponseError(w, util.StatusInternalError(msg, nil))
		return
	}
	defer r.Body.Close()

	if len(new.Spec.Rules) > 1 {
		msg := fmt.Sprintf(`spec.rules cannot have more than one host, found %d rules`, len(new.Spec.Rules))
		glog.V(3).Infof("%s - %s", key, msg)
		details := &metav1.StatusDetails{
			Name:  new.Name,
			Group: new.GroupVersionKind().Group,
			Causes: []metav1.StatusCause{{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: msg,
				Field:   fmt.Sprintf(`spec.rules[%d].host`, len(new.Spec.Rules)),
			}},
		}
		util.WriteResponseError(w, util.StatusConflict(msg, new, details))
		return
	}

	// for now, only care for .spec.rules.http
	new.Spec.Backend = nil
	new.Spec.TLS = []v1beta1.IngressTLS{}

	// validate if the services exists before creating it
	for _, r := range new.Spec.Rules {
		if r.HTTP == nil {
			continue
		}
		for _, p := range r.HTTP.Paths {
			if errStatus := h.validateService(new, &p.Backend); errStatus != nil {
				glog.V(3).Infof("%s - %s", key, errStatus.Message)
				util.WriteResponseError(w, errStatus)
				return
			}
		}
	}

	resp, err := h.usrClientset.Extensions().Ingresses(namespace).Create(new.GetObject())
	switch e := err.(type) {
	case *apierrors.StatusError:
		glog.Infof("%s:%s - failed creating ingress [%s]", key, new.Name, e.ErrStatus.Reason)
		util.WriteResponseError(w, &e.ErrStatus)
	case nil:
		obj, err := runtime.Encode(extensionsCodec, resp)
		if err != nil {
			msg := fmt.Sprintf("request was mutated but failed encoding response [%v]", err)
			glog.Infof("%s:%s - %s", key, new.Name, msg)
			util.WriteResponseError(w, util.StatusInternalError(msg, new))
			return
		}
		glog.Infof("%s:%s - request mutate with success", key, new.Name)
		util.WriteResponseCreated(w, obj)
	default:
		msg := fmt.Sprintf("unknown response from server [%v, %#v]", err, resp)
		glog.Warningf("%s:%s - %s", key, new.Name, msg)
		util.WriteResponseError(w, util.StatusInternalError(msg, new))
		return
	}
}

func (h *Handler) IngressOnPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PATCH" {
		msg := fmt.Sprintf(`Method "%s" not allowed.`, r.Method)
		util.WriteResponseError(w, util.StatusMethodNotAllowed(msg, nil))
		return
	}
	params := mux.Vars(r)
	namespace, ingressName := params["namespace"], params["name"]
	key := fmt.Sprintf("Req-ID=%s, Resource=ingress:%s/%s", r.Header.Get("X-Request-ID"), namespace, ingressName)
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)

	old, errStatus := h.getIngress(namespace, ingressName)
	if errStatus != nil {
		glog.V(4).Infof("%s - failed retrieving ingress [%s]", key, errStatus.Message)
		util.WriteResponseError(w, errStatus)
		return
	}
	new, err := old.DeepCopy()
	if err != nil {
		msg := fmt.Sprintf("failed deep copying obj [%v]", err)
		glog.V(3).Infof("%s -  %s", key, err)
		util.WriteResponseError(w, util.StatusInternalError(msg, nil))
		return
	}

	if err := util.NewDecoder(r.Body, extensionsCodec).Decode(new); err != nil {
		msg := fmt.Sprintf("failed decoding request body [%v]", err)
		glog.V(3).Infof("%s -  %s", key, err)
		util.WriteResponseError(w, util.StatusInternalError(msg, nil))
		return
	}
	oldParent := old.GetAnnotation("kolihub.io/parent")
	if oldParent.Exists() {
		new.SetAnnotation("kolihub.io/parent", oldParent.String())
	}

	// kolihub.io/domain.tld keys are immutable
	for key, value := range old.DomainPrimaryKeys() {
		new.SetAnnotation(key, value)
	}

	// for now, we only care for .spec.rules.http
	new.Spec.Backend = old.Spec.Backend
	new.Spec.TLS = old.Spec.TLS

	if len(new.Spec.Rules) > 1 {
		msg := fmt.Sprintf("spec.rules cannot have more than one host, found %d rules", len(new.Spec.Rules))
		util.WriteResponseError(w, ruleConstraintError(new.GetObject(), msg))
		return
	}

	if len(old.Spec.Rules) == 1 && len(new.Spec.Rules) <= 0 {
		util.WriteResponseError(w, ruleConstraintError(new.GetObject(), "spec.rules cannot be removed"))
		return
	}

	newIngressPaths := map[string]*v1beta1.HTTPIngressPath{}
	for _, r := range new.Spec.Rules {
		if len(old.Spec.Rules) == 1 && r.Host != old.Spec.Rules[0].Host {
			msg := fmt.Sprintf(`cannot change host, from "%s" to "%s"`, old.Spec.Rules[0].Host, r.Host)
			util.WriteResponseError(w, changeHostConstraintError(new.GetObject(), msg))
			return
		}
		for _, p := range r.HTTP.Paths {
			newIngressPaths[p.Path] = &p
		}
	}

	// Try to identify for additions, ensure that the service exists and it's valid
	for newPath, httpIngressPath := range newIngressPaths {
		exists := false
		for _, r := range old.Spec.Rules {
			for _, p := range r.HTTP.Paths {
				if p.Path == newPath {
					exists = true
					break
				}
			}
		}
		if !exists {
			// The new path doesn't exists in the old resource, means
			// it's being added, validate if the service exists before proceed
			if errStatus := h.validateService(new, &httpIngressPath.Backend); errStatus != nil {
				glog.V(3).Infof("%s - %s", key, errStatus.Message)
				util.WriteResponseError(w, errStatus)
				return
			}
		}
	}

	// Try to identify if a path was edited or deleted from the new resource
	for _, r := range old.Spec.Rules {
		for _, p := range r.HTTP.Paths {
			if _, ok := newIngressPaths[p.Path]; !ok {
				// An empty path or root one (/) has no distinction in Kong.
				// Normalize the path otherwise it will generate a distinct adler hash
				pathURI := p.Path
				if pathURI == "/" || pathURI == "" {
					pathURI = "/"
				}

				// The path doesn't exists anymore, means it was removed or
				// edited to a new path value, in this stage is safe to delete the kong route
				if errStatus := h.deleteKongRoute(r.Host, new.Namespace, util.GenAdler32Hash(pathURI)); errStatus != nil {
					glog.V(3).Infof("%s - %s", key, errStatus.Message)
					util.WriteResponseError(w, errStatus)
					return
				}
			}
		}
	}
	// Remove empty keys from map[string]string, it's required because
	// a strategic merge is decoded to an object and every directive is lost.
	// A directive for removing a key from a map[string]string is converted to
	// []byte(`{"metadata": {"labels": "key": ""}}`) and these are not removed
	// when reapplying a merge patch.
	util.DeleteNullKeysFromObjectMeta(&new.ObjectMeta)
	patch, err := util.StrategicMergePatch(extensionsCodec, old.GetObject(), new.GetObject())
	if err != nil {
		msg := fmt.Sprintf("failed merging patch data [%v]", err)
		glog.V(3).Infof("%s -  %s", key, err)
		util.WriteResponseError(w, util.StatusInternalError(msg, nil))
		return
	}

	glog.V(4).Infof("%s, DIFF: %s", key, string(patch))
	resp, err := h.usrClientset.Extensions().Ingresses(namespace).Patch(ingressName, types.StrategicMergePatchType, patch)
	switch e := err.(type) {
	case *apierrors.StatusError:
		glog.Infof("%s - failed updating ingress [%s]", key, e.ErrStatus.Reason)
		util.WriteResponseError(w, &e.ErrStatus)
	case nil:
		data, err := runtime.Encode(extensionsCodec, resp)
		if err != nil {
			msg := fmt.Sprintf("failed encoding response [%v]", err)
			util.WriteResponseError(w, util.StatusInternalError(msg, resp))
			return
		}
		glog.Infof("%s - request mutate with success", key)
		util.WriteResponseSuccess(w, data)
	default:
		msg := fmt.Sprintf("unknown response from server [%v, %#v]", err, resp)
		glog.Warningf("%s - %s", key, msg)
		util.WriteResponseError(w, util.StatusInternalError(msg, resp))
		return
	}
}

func (h *Handler) IngressOnDelete(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, ingressName := params["namespace"], params["name"]
	key := fmt.Sprintf("Req-ID=%s, Resource=ingress:%s/%s", r.Header.Get("X-Request-ID"), namespace, ingressName)
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)

	ing, errStatus := h.getIngress(namespace, ingressName)
	if errStatus != nil {
		glog.V(4).Infof("%s - failed retrieving ingress [%s]", key, errStatus.Message)
		util.WriteResponseError(w, errStatus)
		return
	}

	domainName := ""
	if len(ing.Spec.Rules) != 1 {
		domainName = ing.Spec.Rules[0].Host
	}

	if len(domainName) == 0 {
		glog.Infof("%s - empty domain found, noop", key)
		util.WriteResponseNoContent(w)
	}

	resp, err := h.usrTprClient.Delete().
		Resource("domains").
		Namespace(namespace).
		Name(domainName).
		DoRaw()
	switch e := err.(type) {
	case *apierrors.StatusError:
		glog.Infof("%s -  failed mutating request [%v, %s]", key, err, string(resp))
		util.WriteResponseError(w, &e.ErrStatus)
	case nil:
		glog.Infof("%s -  request mutate with success!", key)
		util.WriteResponseNoContent(w)
	default:
		msg := fmt.Sprintf("unknown response [%v, %s]", err, string(resp))
		glog.Infof("%s -  %s", key, msg)
		util.WriteResponseError(w, util.StatusInternalError(msg, &platform.Domain{}))
	}
}

func (h *Handler) deleteKongRoute(host, ns, encodedPath string) *metav1.Status {
	apiName := fmt.Sprintf("%s~%s~%s", host, ns, encodedPath)
	glog.V(4).Infof(`removing kong route "%s"`, apiName)
	res := h.kongClient.Delete().
		Resource("apis").
		Name(apiName).
		Do()
	if res.StatusCode() == http.StatusNotFound {
		glog.V(3).Infof(`kong api "%s" doesn't exists`, apiName)
		return nil
	}
	if err := res.Error(); err != nil {
		return util.StatusBadRequest(fmt.Sprintf("failed removing kong route [%v]", err), nil, metav1.StatusReasonBadRequest)
	}
	return nil
}

func (h *Handler) validateService(ing *draft.Ingress, b *v1beta1.IngressBackend) *metav1.Status {
	glog.V(4).Infof(`validating service [%#v]`, b)
	svc, err := h.clientset.Core().Services(ing.Namespace).Get(b.ServiceName, metav1.GetOptions{})
	if err != nil {
		return util.StatusBadRequest(fmt.Sprintf("failed retrieving service [%v]", err), nil, metav1.StatusReasonBadRequest)
	}
	portExists := false
	for _, port := range svc.Spec.Ports {
		if port.Port == b.ServicePort.IntVal {
			portExists = true
			break
		}
	}
	if !portExists {
		msg := fmt.Sprintf(`the service port "%d" doesn't exists in service "%s", found: %#v`, b.ServicePort.IntVal, svc.Name, svc.Spec.Ports)
		return ingressContraintError(ing.GetObject(), msg, "spec.rules[0].http[?].paths[?].backend.servicePort", metav1.CauseTypeFieldValueInvalid)
	}
	return nil
}

func (h *Handler) getIngress(namespace, ingName string) (*draft.Ingress, *metav1.Status) {
	ing, err := h.clientset.Extensions().Ingresses(namespace).Get(ingName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf(`ingress "%s" not found`, ingName)
			return nil, util.StatusNotFound(msg, nil)
		}
		msg := fmt.Sprintf("failed retrieving ingress [%v]", err)
		return nil, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest)
	}
	return draft.NewIngress(ing), nil
}

func ruleConstraintError(ing *v1beta1.Ingress, msg string) *metav1.Status {
	field := fmt.Sprintf("spec.rules[%d].host", len(ing.Spec.Rules))
	return ingressContraintError(ing, msg, field, metav1.CauseTypeFieldValueInvalid)
}

func changeHostConstraintError(ing *v1beta1.Ingress, msg string) *metav1.Status {
	return ingressContraintError(ing, msg, "spec.rules[0].host", metav1.CauseTypeFieldValueNotSupported)
}

func ingressContraintError(ing *v1beta1.Ingress, msg, field string, cause metav1.CauseType) *metav1.Status {
	details := &metav1.StatusDetails{
		Name:  ing.Name,
		Group: ing.GroupVersionKind().Group,
		Causes: []metav1.StatusCause{{
			Type:    cause,
			Message: msg,
			Field:   field,
		}},
	}
	return util.StatusUnprocessableEntity(msg, ing, details)
}

// func (h *Handler) isDomainClaimed(ing *v1beta1.Ingress) (bool, error) {
// 	if len(ing.Spec.Rules) != 1 {
// 		return false, fmt.Errorf("ingress must contain only one host, found %d", len(ing.Spec.Rules))
// 	}
// 	ingRule := ing.Spec.Rules[0]
// 	if ing.Annotations[fmt.Sprintf("kolihub.io/%s", ingRule.Host)] == "primary" {
// 		domList, err := h.fetchDomainList()
// 		if err != nil {
// 			return false, fmt.Errorf("failed fetching domain list, %v", err)
// 		}
// 		for _, d := range domList.Items {
// 			if !d.IsPrimary() || !d.IsOK() {
// 				continue
// 			}
// 			if d.Name == ingRule.Host && d.Namespace != ing.Namespace {
// 				return true, fmt.Errorf(`domain "%s" is claimed at "%s"`, d.Name, d.Namespace)
// 			}
// 		}
// 		return false, nil
// 	}

// 	domList, err := h.fetchDomainList()
// 	if err != nil {
// 		return false, fmt.Errorf("failed fetching domain list, %v", err)
// 	}
// 	for _, d := range domList.Items {
// 		if d.IsPrimary() || !d.IsOK() {
// 			continue
// 		}
// 		if d.Name == ingRule.Host {
// 			return true,
// 		}
// 	}
// }

// func (h *Handler) fetchDomainList() (*platform.DomainList, error) {
// 	var domList *platform.DomainList
// 	err := h.tprClient.Get().
// 		Namespace(metav1.NamespaceAll).
// 		Resource("domains").
// 		Do().
// 		Into(domList)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return domList, nil
// }

// // isPrimaryDomainClaimed verifies if a platform.Domain is claimed in the system namespace
// // or in the current namespace of the ingress resource
// func (h *Handler) isPrimaryDomainClaimed() (bool, error) {
// 	return false, nil
// }
