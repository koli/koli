package mutator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	platform "kolihub.io/koli/pkg/spec"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	kongFinalizer = "kolihub.io/kong"
)

func (h *Handler) isValidResource(obj *platform.Domain) *metav1.Status {
	if !obj.IsValidDomain() {
		msg := "invalid resource, attribute spec.primary must be a valid fully qualified domain, " +
			"spec.sub if specified, must be a subdomain name of the spec.primary"
		details := &metav1.StatusDetails{
			Name:  obj.Name,
			Group: obj.GroupVersionKind().Group,
			Causes: []metav1.StatusCause{{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Field:   "spec.primary or spec.sub",
				Message: msg,
			}},
		}
		return StatusUnprocessableEntity(msg, obj, details)
	}
	return nil
}

// DomainsOnCreate validate and mutates POST requests
func (h *Handler) DomainsOnCreate(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	key := fmt.Sprintf("Req-ID=%s, Resource=domains", r.Header.Get("X-Request-ID"))
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)
	new := &platform.Domain{}
	if err := json.NewDecoder(r.Body).Decode(new); err != nil {
		msg := fmt.Sprintf("failed decoding request body [%v]", err)
		glog.V(3).Infof("%s -  %s", key, msg)
		writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
		return
	}
	defer r.Body.Close()
	key = fmt.Sprintf("%s:%s/%s", key, new.Namespace, new.Name)
	if errStatus := h.isValidResource(new); errStatus != nil {
		glog.V(3).Infof("%s -  %s", key, errStatus.Message)
		writeResponseError(w, errStatus)
		return
	}
	domList := &platform.DomainList{}
	err := h.tprClient.Get().
		Resource("domains").
		Do().
		Into(domList)
	if err != nil {
		msg := fmt.Sprintf("failed retrieving domain list [%v]", err)
		glog.V(3).Infof("%s -  %s", key, msg)
		writeResponseError(w, StatusInternalError(msg, new))
		return
	}

	for _, cur := range domList.Items {
		if new.IsPrimary() {
			if cur.IsPrimary() && cur.GetPrimaryDomain() == new.GetPrimaryDomain() {
				msg := fmt.Sprintf("primary domain already exist at namespace/resource [%s/%s]", new.Namespace, new.Name)
				glog.V(3).Infof("%s -  %s", key, msg)
				writeResponseError(w, StatusConflict(msg, new, nil))
				return
			}
		} else {
			if !cur.IsPrimary() && cur.GetDomain() == new.GetDomain() {
				msg := fmt.Sprintf("domain already exist at namespace/resource [%s/%s]", new.Namespace, new.Name)
				glog.V(3).Infof("%s -  %s", key, msg)
				writeResponseError(w, StatusConflict(msg, new, nil))
				return
			}
		}
	}
	if len(new.Spec.Parent) > 0 && !new.IsPrimary() {
		if errStatus := h.validateIfParentIsValid(new); errStatus != nil {
			glog.V(3).Infof("%s - %v", key, errStatus.Message)
			writeResponseError(w, errStatus)
			return
		}
	}

	// an user couldn't change the status!
	new.Status = platform.DomainStatus{}
	resp, err := h.usrTprClient.Post().
		Resource("domains").
		Namespace(params["namespace"]).
		Body(new).
		DoRaw()
	switch e := err.(type) {
	case *apierrors.StatusError:
		e.ErrStatus.APIVersion = new.APIVersion
		e.ErrStatus.Kind = "Status"
		glog.Infof("%s -  failed mutating request [%v]", key, err)
		writeResponseError(w, &e.ErrStatus)
	case nil:
		glog.Infof("%s -  request mutate with success", key)
		writeResponseCreated(w, resp)
	default:
		msg := fmt.Sprintf("unknown response [%v, %s]", err, string(resp))
		glog.Infof("%s -  %s", key, msg)
		writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
	}
}

// DomainsOnMod mutates and validates PUT and PATCH requests
func (h *Handler) DomainsOnMod(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	key := fmt.Sprintf("Req-ID=%s, Resource=domains:%s/%s", r.Header.Get("X-Request-ID"), params["namespace"], params["domain"])
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)
	switch r.Method {
	case "PUT":
		new := &platform.Domain{}
		if err := json.NewDecoder(r.Body).Decode(new); err != nil {
			msg := fmt.Sprintf("failed decoding request body [%v]", err)
			writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
			return
		}
		defer r.Body.Close()
		if errStatus := h.isValidResource(new); errStatus != nil {
			glog.Infof("%s - %s", key, errStatus.Details.Causes[0].Message)
			writeResponseError(w, errStatus)
			return
		}

		old, errStatus := h.getDomainByName(params["namespace"], params["domain"])
		if errStatus != nil {
			glog.Infof("%s -  %s", key, errStatus.Message)
			writeResponseError(w, errStatus)
			return
		}
		if old.Spec.PrimaryDomain != new.Spec.PrimaryDomain || old.Spec.Sub != new.Spec.Sub {
			msg := "not allowed to change spec.primary or spec.sub"
			glog.Infof("%s -  %s", key, msg)
			details := &metav1.StatusDetails{
				Name:  old.Name,
				Group: old.GroupVersionKind().Group,
				Causes: []metav1.StatusCause{{
					Type:    metav1.CauseTypeFieldValueNotSupported,
					Message: msg,
					Field:   "spec.primary or spec.sub",
				}},
			}
			writeResponseError(w, StatusUnprocessableEntity(msg, old, details))
			return
		}

		if errStatus := h.validateIfDelegatesHasChanged(old, new.Spec.Delegates); errStatus != nil {
			glog.Infof("%s -  %s", key, errStatus.Message)
			writeResponseError(w, errStatus)
			return
		}

		if len(new.Spec.Parent) > 0 && old.Spec.Parent != new.Spec.Parent && !old.IsPrimary() {
			if errStatus := h.validateIfParentIsValid(new); errStatus != nil {
				glog.V(3).Infof("%s - %s", key, errStatus.Message)
				writeResponseError(w, errStatus)
				return
			}
		}

		// an user couldn't change the status!
		new.Status = old.Status
		new.Finalizers = old.Finalizers
		resp, err := h.usrTprClient.Put().
			Resource("domains").
			Namespace(params["namespace"]).
			Name(params["domain"]).
			Body(new).
			DoRaw()
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = old.APIVersion
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s -  failed mutating request [%v, %s]", key, err, string(resp))
			writeResponseError(w, &e.ErrStatus)
		case nil:
			glog.Infof("%s -  request mutate with success!", key)
			writeResponseSuccess(w, resp)
		default:
			msg := fmt.Sprintf("unknown response [%v, %s]", err, string(resp))
			glog.Infof("%s -  %s", key, msg)
			writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
		}
	case "PATCH":
		var obj map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
			msg := fmt.Sprintf("failed decoding request body [%v]", err)
			glog.V(3).Infof("%s -  %s", key, err)
			writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
			return
		}
		if _, ok := obj["spec"]; ok {
			objSpec := obj["spec"].(map[string]interface{})
			old, errStatus := h.getDomainByName(params["namespace"], params["domain"])
			if errStatus != nil {
				glog.Infof("%s -  %s", key, errStatus.Message)
				writeResponseError(w, errStatus)
				return
			}
			for specKey, val := range objSpec {
				switch specKey {
				case "parent":
					// the user is removing the key, let the
					// kong ingress controller deal with it
					if val == nil {
						continue
					}
					parent := reflect.ValueOf(val).String()
					if len(parent) > 0 && !old.IsPrimary() {
						old.Spec.Parent = parent
						if errStatus := h.validateIfParentIsValid(old); errStatus != nil {
							glog.Infof("%s -  %s", key, errStatus.Message)
							writeResponseError(w, errStatus)
							return
						}
					}
				case "delegates":
					// the key was removed, it's an empty slice
					if val == nil {
						val = []string{}
					}
					if reflect.TypeOf(val).Kind() != reflect.Slice {
						msg := fmt.Sprintf("found wrong type [%T], expected [slice]", val)
						glog.Infof("%s -  %s", key, msg)
						details := &metav1.StatusDetails{
							Name:  old.Name,
							Group: old.GroupVersionKind().Group,
							Causes: []metav1.StatusCause{{
								Type:    metav1.CauseTypeFieldValueInvalid,
								Message: msg,
								Field:   "spec.primary or spec.sub",
							}},
						}
						writeResponseError(w, StatusUnprocessableEntity(msg, &platform.Domain{}, details))
						return
					}
					var delegates []string
					s := reflect.ValueOf(val)

					for i := 0; i < s.Len(); i++ {
						d := fmt.Sprintf("%v", s.Index(i).Interface())
						delegates = append(delegates, d)
					}
					if errStatus := h.validateIfDelegatesHasChanged(old, delegates); errStatus != nil {
						glog.Infof("%s -  %s", key, errStatus.Message)
						writeResponseError(w, errStatus)
						return
					}
				default:
					msg := "not allowed to change spec.primary or spec.sub"
					glog.Infof("%s -  %s", key, msg)
					details := &metav1.StatusDetails{
						Name:  old.Name,
						Group: old.GroupVersionKind().Group,
						Causes: []metav1.StatusCause{{
							Type:    metav1.CauseTypeFieldValueNotSupported,
							Message: msg,
							Field:   "spec.primary or spec.sub",
						}},
					}
					writeResponseError(w, StatusUnprocessableEntity(msg, old, details))
					return
				}
			}
		}
		// could not change the status spec, remove it
		delete(obj, "status")
		reqBody, err := json.Marshal(obj)
		if err != nil {
			msg := fmt.Sprintf("failed encoding request body [%v]", err)
			glog.V(3).Infof("%s -  %s", key, msg)
			writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
			return
		}
		resp, err := h.usrTprClient.Patch(types.MergePatchType).
			Resource("domains").
			Namespace(params["namespace"]).
			Name(params["domain"]).
			Body(reqBody).
			DoRaw()
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = platform.SchemeGroupVersion.Version
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s -  failed mutating request [%v, %s]", key, err, string(resp))
			writeResponseError(w, &e.ErrStatus)
		case nil:
			glog.Infof("%s -  request mutate with success!", key)
			writeResponseCreated(w, resp)
		default:
			msg := fmt.Sprintf("unknown response [%v, %s]", err, string(resp))
			glog.Infof("%s -  %s", key, msg)
			writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
		}
	case "DELETE":
		old, errStatus := h.getDomainByName(params["namespace"], params["domain"])
		if errStatus != nil {
			glog.Infof("%s -  %s", key, errStatus.Message)
			writeResponseError(w, errStatus)
			return
		}

		if old.HasFinalizer(kongFinalizer) {
			glog.V(3).Infof("%s -  found finalizer %s, configuring delete timestamp", key, kongFinalizer)
			nowUTC := metav1.Now().UTC().Format(time.RFC3339)
			reqBody := []byte(fmt.Sprintf(`{"status": {"deletionTimestamp": "%s"}}`, nowUTC))
			resp, err := h.usrTprClient.Patch(types.MergePatchType).
				Resource("domains").
				Namespace(params["namespace"]).
				Name(params["domain"]).
				Body(reqBody).
				DoRaw()
			switch e := err.(type) {
			case *apierrors.StatusError:
				e.ErrStatus.APIVersion = platform.SchemeGroupVersion.Version
				e.ErrStatus.Kind = "Status"
				glog.Infof("%s -  failed mutating request [%v, %s]", key, err, string(resp))
				writeResponseError(w, &e.ErrStatus)
			case nil:
				glog.Infof("%s -  request mutate with success!", key)
				writeResponseNoContent(w)
			default:
				msg := fmt.Sprintf("unknown response [%v, %s]", err, string(resp))
				glog.Infof("%s -  %s", key, msg)
				writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
			}
			return
		}
		resp, err := h.usrTprClient.Delete().
			Resource("domains").
			Namespace(params["namespace"]).
			Name(params["domain"]).
			DoRaw()
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = platform.SchemeGroupVersion.Version
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s -  failed mutating request [%v, %s]", key, err, string(resp))
			writeResponseError(w, &e.ErrStatus)
		case nil:
			glog.Infof("%s -  request mutate with success!", key)
			writeResponseNoContent(w)
		default:
			msg := fmt.Sprintf("unknown response [%v, %s]", err, string(resp))
			glog.Infof("%s -  %s", key, msg)
			writeResponseError(w, StatusInternalError(msg, &platform.Domain{}))
		}

	default:
		writeResponseError(w, StatusMethodNotAllowed("Method Not Allowed.", &platform.Domain{}))
	}
}

// validateIfDelegatesHasChanged verifies if the 'delegates' attribute from an old resource
// contains all the namespaces in the new one. If a namespace is missing, verify if
// there's an associated parent.
func (h *Handler) validateIfDelegatesHasChanged(old *platform.Domain, newDelegates []string) *metav1.Status {
	if !old.IsPrimary() && len(old.Spec.Delegates) == 0 && reflect.DeepEqual(old.Spec.Delegates, newDelegates) {
		return nil
	}
	// search and add for all namespaces deleted based on the
	// newDelegates slice
	var delNamespaces []string
	for _, o := range old.Spec.Delegates {
		exists := false
		for _, n := range newDelegates {
			if o == n {
				exists = true
				break
			}
		}
		if !exists {
			delNamespaces = append(delNamespaces, o)
		}
	}
	if len(delNamespaces) == 0 {
		return nil
	}
	domList := &platform.DomainList{}
	if err := h.tprClient.Get().
		Resource("domains").
		Do().
		Into(domList); err != nil {
		msg := fmt.Sprintf("failed retrieving domain list [%s]", err)
		return StatusBadRequest(msg, old, metav1.StatusReasonUnknown)
	}

	// search if the deleted namespaces are associated with shared domains in the cluster
	for _, cur := range domList.Items {
		if cur.IsPrimary() && !cur.IsOK() {
			continue
		}
		for _, delns := range delNamespaces {
			if delns == "*" && cur.GetPrimaryDomain() == old.GetPrimaryDomain() ||
				delns == cur.Namespace && cur.Spec.Parent == old.Namespace {
				// skip own resource
				if cur.Name == old.Name && cur.Namespace == old.Namespace {
					continue
				}
				msg := fmt.Sprintf("found an associated valid domain claim[%s] at namespace/resource [%s/%s]",
					cur.GetDomain(), cur.Namespace, cur.Name)
				return StatusConflict(msg, old, nil)
			}
		}
	}
	return nil
}

// validateIfParentIsValid verify if the parent namespace exists and if it's delegating to it
func (h *Handler) validateIfParentIsValid(obj *platform.Domain) *metav1.Status {
	domList := &platform.DomainList{}
	err := h.tprClient.Get().
		Resource("domains").
		Namespace(obj.Spec.Parent).
		Do().
		Into(domList)
	if err != nil {
		msg := fmt.Sprintf("failed retrieving domain list [%v]", err)
		return StatusBadRequest(msg, obj, metav1.StatusReasonUnknown)
	}
	isAllowed := false
	for _, d := range domList.Items {
		if d.IsPrimary() && d.IsOK() && d.GetPrimaryDomain() == obj.GetPrimaryDomain() {
			// if the domain is in the same namespace, it's allowed to claim
			if d.Namespace == obj.Namespace {
				isAllowed = true
				break
			}

			// if the domain doesn't belong to the same namespace,
			// validate if the domain is delagating to its parent
			if d.Namespace == obj.Spec.Parent && d.HasDelegate(obj.Namespace) {
				isAllowed = true
				break
			}
		}
	}
	if !isAllowed {
		msg := "the parent namespace wasn't found or allowed to claim"
		details := &metav1.StatusDetails{
			Name:  obj.Name,
			Group: obj.GroupVersionKind().Group,
			Causes: []metav1.StatusCause{{
				Type:    metav1.CauseTypeFieldValueNotFound,
				Field:   "spec.parent",
				Message: msg,
			}},
		}
		return StatusUnprocessableEntity(msg, obj, details)
	}
	return nil
}

func (h *Handler) getDomainByName(namespace, name string) (*platform.Domain, *metav1.Status) {
	obj := &platform.Domain{}
	err := h.usrTprClient.Get().
		Resource("domains").
		Namespace(namespace).
		Name(name).
		Do().
		Into(obj)
	if err != nil {
		switch t := err.(type) {
		case apierrors.APIStatus:
			if t.Status().Reason == metav1.StatusReasonNotFound {
				return nil, StatusNotFound(fmt.Sprintf("domain \"%s\" not found", name), obj)
			}
		}
		return nil, StatusInternalError(fmt.Sprintf("unknown error retrieving namespace [%v]", err), obj)
	}
	return obj, nil
}
