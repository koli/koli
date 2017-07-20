package mutator

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/api/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/apis/v1alpha1/draft"
	"kolihub.io/koli/pkg/util"
)

const (
	// NamespaceIsolationAnnotation deny traffic between pods
	// https://kubernetes.io/docs/concepts/services-networking/networkpolicies/#configuring-namespace-isolation
	NamespaceIsolationAnnotation = "net.beta.kubernetes.io/network-policy"
)

func invalidNamespaceDetails(ns *v1.Namespace) *metav1.Status {
	msg := fmt.Sprintf("Invalid value: \"%s\": it must consist of lower case ", ns.Name)
	msg = msg + "alphanumeric characters, and must start and end with an " +
		"alphanumeric character. A hiffen \"-\", must be used to prefix the namespace also " +
		"(e.g. 'myname-coyote-acme')"
	details := &metav1.StatusDetails{
		Name: ns.Name,
		Kind: ns.GroupVersionKind().Kind,
		Causes: []metav1.StatusCause{{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: msg,
			Field:   "metadata.name",
		}},
	}
	return util.StatusUnprocessableEntity(msg, ns, details)
}

func (h *Handler) NamespaceOnList(w http.ResponseWriter, r *http.Request) {
	key := fmt.Sprintf("Req-ID=%s, Resource=namespaces", r.Header.Get("X-Request-ID"))
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)

	selector := labels.Set{
		platform.LabelCustomer:     h.user.Customer,
		platform.LabelOrganization: h.user.Organization,
	}
	nsList, err := h.usrClientset.Core().Namespaces().List(metav1.ListOptions{LabelSelector: selector.String()})
	switch e := err.(type) {
	case *apierrors.StatusError:
		e.ErrStatus.APIVersion = nsList.APIVersion
		e.ErrStatus.Kind = "Status"
		glog.Infof("%s - failed listing namespace [%s]", key, e.ErrStatus.Reason)
		util.WriteResponseError(w, &e.ErrStatus)
	case nil:
		data, err := runtime.Encode(scheme.Codecs.LegacyCodec(v1.SchemeGroupVersion), nsList)
		if err != nil {
			msg := fmt.Sprintf("failed encoding response [%v]", err)
			glog.Infof("%s - %s", key, msg)
			util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
			return
		}
		glog.Infof("%s - request mutate with success", key)
		util.WriteResponseCreated(w, data)
	default:
		msg := fmt.Sprintf("unknown response from server [%v, %#v]", err, nsList)
		glog.Warningf("%s - %s", key, msg)
		util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
		return
	}
}

// NamespaceOnCreate mutates k8s request on creation
func (h *Handler) NamespaceOnCreate(w http.ResponseWriter, r *http.Request) {
	key := fmt.Sprintf("Req-ID=%s, Resource=namespaces", r.Header.Get("X-Request-ID"))
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)

	new := &v1.Namespace{}
	if err := json.NewDecoder(r.Body).Decode(new); err != nil {
		msg := fmt.Sprintf("failed decoding request body [%v]", err)
		glog.Errorf("%s - %s", key, msg)
		util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
		return
	}
	defer r.Body.Close()

	nsMeta := draft.NewNamespaceMetadata(new.Name)
	if !nsMeta.IsValid() {
		glog.V(4).Infof("%s - invalid namespace format", key)
		util.WriteResponseError(w, invalidNamespaceDetails(new))
		return
	}

	if h.user.Customer != nsMeta.Customer() || h.user.Organization != nsMeta.Organization() {
		msg := forbiddenAccessMessage(h.user, nsMeta.Customer(), nsMeta.Organization())
		util.WriteResponseError(w, util.StatusUnauthorized(msg, new, metav1.StatusReasonUnauthorized))
		return
	}

	initializeMetadata(&new.ObjectMeta)

	// Traffic between namespaces are not allowed by default
	new.Annotations[NamespaceIsolationAnnotation] = `{"ingress": {"isolation": "DefaultDeny"}}`
	new.Annotations[platform.AnnotationNamespaceOwner] = h.user.Email

	// Allow kong to access services from all namespaces in the platform.
	// It works only if there's a specific network policy on the target namespace,
	// which is provisioned by the controller.
	new.Labels[platform.LabelAllowKongTraffic] = "true"
	new.Labels[platform.LabelCustomer] = h.user.Customer
	new.Labels[platform.LabelOrganization] = h.user.Organization

	if r.Method == "POST" {
		resp, err := h.usrClientset.Core().Namespaces().Create(new)
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = new.APIVersion
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s:%s - failed creating namespace [%s]", key, new.Name, e.ErrStatus.Reason)
			util.WriteResponseError(w, &e.ErrStatus)
		case nil:
			resp.Kind = new.Kind
			resp.APIVersion = new.APIVersion
			data, err := json.Marshal(resp)
			if err != nil {
				msg := fmt.Sprintf("failed encoding response [%v]", err)
				glog.Infof("%s:%s - %s", key, new.Name, msg)
				util.WriteResponseError(w, util.StatusInternalError(msg, new))
				return
			}
			glog.Infof("%s:%s - request mutate with success", key, new.Name)
			util.WriteResponseCreated(w, data)
		default:
			msg := fmt.Sprintf("unknown response from server [%v, %#v]", err, resp)
			glog.Warningf("%s:%s - %s", key, new.Name, msg)
			util.WriteResponseError(w, util.StatusInternalError(msg, new))
			return
		}
	}
}

// NamespaceOnMod mutates k8s request on modify http methods (PUT, PATCH)
func (h *Handler) NamespaceOnMod(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	key := fmt.Sprintf("Req-ID=%s, Resource=namespaces:%s", r.Header.Get("X-Request-ID"), params["name"])
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)
	switch r.Method {
	case "PUT":
		new := &v1.Namespace{}
		if err := json.NewDecoder(r.Body).Decode(new); err != nil {
			msg := fmt.Sprintf("failed decoding request body [%v]", err)
			glog.Errorf("%s - %s", key, msg)
			util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
			return
		}
		defer r.Body.Close()
		old, errStatus := h.getNamespace(params["name"])
		if errStatus != nil {
			glog.V(4).Infof("%s - failed retrieving namespace [%s]", key, errStatus.Message)
			util.WriteResponseError(w, errStatus)
			return
		}

		initializeMetadata(&new.ObjectMeta)
		// immutable keys (from an user perspective)
		new.Labels[platform.LabelAllowKongTraffic] = old.Labels[platform.LabelAllowKongTraffic]
		new.Annotations[platform.AnnotationNamespaceOwner] = old.Annotations[platform.AnnotationNamespaceOwner]
		new.Annotations[NamespaceIsolationAnnotation] = old.Annotations[NamespaceIsolationAnnotation]

		// immutable keys (from an user perspective)
		new.Labels[platform.LabelCustomer] = old.Labels[platform.LabelCustomer]
		new.Labels[platform.LabelOrganization] = old.Labels[platform.LabelOrganization]

		resp, err := h.usrClientset.Core().Namespaces().Update(new)
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = resp.APIVersion
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s - failed updating namespace [%s]", key, e.ErrStatus.Reason)
			util.WriteResponseError(w, &e.ErrStatus)
		case nil:
			resp.Kind = new.Kind
			resp.APIVersion = new.APIVersion
			data, err := json.Marshal(resp)
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
	case "PATCH":
		var new map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&new); err != nil {
			msg := fmt.Sprintf("failed decoding request body [%v]", err)
			util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
			return
		}
		defer r.Body.Close()
		old, errStatus := h.getNamespace(params["name"])
		if errStatus != nil {
			util.WriteResponseError(w, errStatus)
			return
		}

		if _, ok := new["metadata"]; ok {
			meta := new["metadata"].(map[string]interface{})
			if k, ok := meta["annotations"]; ok {
				var annotations map[string]interface{}
				// all annotations were deleted
				if k == nil {
					meta["annotations"] = make(map[string]interface{})
					annotations = meta["annotations"].(map[string]interface{})
					// set all keys as nil to delete then when patching
					for key := range old.Annotations {
						annotations[key] = nil
					}
				}

				annotations = meta["annotations"].(map[string]interface{})
				// immutable keys (from an user perspective)
				annotations[platform.AnnotationNamespaceOwner] = old.Annotations[platform.AnnotationNamespaceOwner]
				annotations[NamespaceIsolationAnnotation] = old.Annotations[NamespaceIsolationAnnotation]

			}
			if k, ok := meta["labels"]; ok {
				var labels map[string]interface{}
				// all labels were deleted
				if k == nil {
					meta["labels"] = make(map[string]interface{})
					labels = meta["labels"].(map[string]interface{})
					// set all keys as nil to delete then when patching
					for key := range old.Labels {
						labels[key] = nil
					}
				}

				labels = meta["labels"].(map[string]interface{})
				// immutable keys (from an user perspective)
				labels[platform.LabelAllowKongTraffic] = old.Labels[platform.LabelAllowKongTraffic]
				labels[platform.LabelCustomer] = old.Labels[platform.LabelCustomer]
				labels[platform.LabelOrganization] = old.Labels[platform.LabelOrganization]
			}
		}

		// fmt.Printf("AFTER: %#v\n", new)
		reqBody, err := json.Marshal(new)
		if err != nil {
			msg := fmt.Sprintf("failed encoding request body [%v]", err)
			glog.Infof("%s - %s", key, msg)
			util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
			return
		}
		resp, err := h.usrClientset.Core().Namespaces().Patch(params["name"], types.MergePatchType, reqBody)
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = resp.APIVersion
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s - failed updating namespace [%s]", key, e.ErrStatus.Reason)
			util.WriteResponseError(w, &e.ErrStatus)
		case nil:
			resp.Kind = "Namespace"
			resp.APIVersion = "v1"
			data, err := json.Marshal(resp)
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
	default:
		util.WriteResponseError(w, util.StatusMethodNotAllowed("Method Not Allowed.", &v1.Namespace{}))
	}
}

func (h *Handler) getNamespace(name string) (*v1.Namespace, *metav1.Status) {
	obj, err := h.usrClientset.Core().Namespaces().Get(name, metav1.GetOptions{})
	if err != nil {
		switch t := err.(type) {
		case apierrors.APIStatus:
			if t.Status().Reason == metav1.StatusReasonNotFound {
				return nil, util.StatusNotFound(fmt.Sprintf("namespace \"%s\" not found", name), &v1.Namespace{})
			}
		}
		return nil, util.StatusInternalError(fmt.Sprintf("unknown error retrieving namespace [%v]", err), &v1.Namespace{})
	}
	return obj, nil
}
