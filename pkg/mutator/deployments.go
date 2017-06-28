package mutator

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	draft "kolihub.io/koli/pkg/apis/v1alpha1/draft"
)

const (
	buildpacksImage = "quay.io/slugrunner"
)

var (
	immutableAnnotations = []string{
		platform.AnnotationAuthToken, platform.AnnotationBuildRevision, platform.AnnotationBuildSource,
		platform.AnnotationGitCompare, platform.AnnotationGitHubSecretHook, platform.AnnotationGitHubUser,
		platform.AnnotationSetupStorage,
	}
)

// DeploymentsOnCreate mutate requests on POST
func (h *Handler) DeploymentsOnCreate(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	key := fmt.Sprintf("Req-ID=%s, Resource=deployments", r.Header.Get("X-Request-ID"))
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)
	new := draft.NewDeployment(&v1beta1.Deployment{})
	if err := json.NewDecoder(r.Body).Decode(new); err != nil {
		msg := fmt.Sprintf("failed decoding request body [%v]", err)
		glog.V(3).Infof("%s -  %s", key, msg)
		writeResponseError(w, StatusInternalError(msg, &v1beta1.Deployment{}))
		return
	}
	defer r.Body.Close()
	key = fmt.Sprintf("%s:%s/%s", key, new.Namespace, new.Name)

	// TODO: validate if the image is allowed!
	planList := &platform.PlanList{}
	if err := h.tprClient.Get().
		Namespace(platform.SystemNamespace).
		Resource("plans").
		Do().
		Into(planList); err != nil {
		msg := fmt.Sprintf("failed retrieving plan list [%v]", err)
		glog.V(3).Infof("%s -  %s", key, msg)
		writeResponseError(w, StatusInternalError(msg, new))
		return
	}

	for _, immutableKey := range immutableAnnotations {
		delete(new.Annotations, immutableKey)
	}

	if errStatus := h.validateContainerImage(new); errStatus != nil {
		glog.Infof("%s - %s", key, errStatus.Message)
		writeResponseError(w, errStatus)
		return
	}

	var plan *platform.Plan
	setupStorage := false
	clusterPlanName := new.GetClusterPlan()
	for i := 0; i < len(planList.Items); i++ {
		if new.GetStoragePlan().String() == planList.Items[i].Name {
			setupStorage = true
			continue
		}

		if planList.Items[i].Name == clusterPlanName.String() {
			plan = &planList.Items[i]
			continue
		}
		// Find a default plan if the deployment doesn't have a specified plan
		if planList.Items[i].Labels != nil && planList.Items[i].Labels[platform.LabelDefault] == "true" {
			// Don't mind if there's more than one resource as default
			plan = &planList.Items[i]
		}
	}
	if plan == nil {
		msg := "the deployment doesn't have a specified plan neither a default one was found"
		if clusterPlanName.Exists() {
			msg = fmt.Sprintf(`plan "%s" not found, neither a default one`, clusterPlanName.String())
		}
		glog.V(3).Infof("%s -  %s", key, msg)
		writeResponseError(w, StatusNotFound(msg, new))
		return
	}

	if !plan.IsDefaultType() {
		msg := fmt.Sprintf(`plan "%s" has a wrong type "%s"`, plan.Name, plan.Spec.Type)
		glog.V(3).Infof("%s - %s", key, msg)
		details := &metav1.StatusDetails{
			Name:  new.Name,
			Group: new.GroupVersionKind().Group,
			Causes: []metav1.StatusCause{{
				Type:    metav1.CauseTypeFieldValueNotSupported,
				Message: msg,
				Field:   fmt.Sprintf(`metadata.labels[%s]`, platform.LabelStoragePlan),
			}},
		}
		writeResponseError(w, StatusConflict(msg, new, details))
		return
	}

	if setupStorage {
		// Don't let scale with persistent volumes
		if new.HasMultipleReplicas() {
			msg := fmt.Sprintf("found a persistent volume, unable to scale")
			glog.Infof("%s:%s - %s", key, new.Name, msg)
			writeResponseError(w, StatusInternalError(msg, new))
			return
		}
		// The controller will catch this state and starts
		// provisioning a storage for this resource
		new.SetAnnotation(platform.AnnotationSetupStorage, "true")
	}

	// allow .spec.template.spec.containers
	defaultsC, newCs := v1.Container{}, new.Spec.Template.Spec.Containers
	if len(newCs) > 0 {
		defaultsC.Name = new.Name
		defaultsC.Args = newCs[0].Args
		defaultsC.Command = newCs[0].Command
		defaultsC.Env = newCs[0].Env
		defaultsC.EnvFrom = newCs[0].EnvFrom
		defaultsC.Image = newCs[0].Image
		defaultsC.Ports = newCs[0].Ports
		defaultsC.VolumeMounts = newCs[0].VolumeMounts
		defaultsC.Resources = plan.Spec.Resources
	}

	// allow .spec.template.metadata
	podTemplateMeta := new.Spec.Template.ObjectMeta

	// Mutate PodTemplateSpec
	new.Spec.Template = v1.PodTemplateSpec{}
	new.Spec.Template.ObjectMeta = podTemplateMeta
	new.Spec.Template.Spec.Containers = []v1.Container{defaultsC}
	new.SetClusterPlan(plan.Name)

	resp, err := h.usrClientset.Extensions().Deployments(params["namespace"]).Create(new.GetObject())
	switch e := err.(type) {
	case *apierrors.StatusError:
		e.ErrStatus.APIVersion = new.APIVersion
		e.ErrStatus.Kind = "Status"
		glog.Infof("%s:%s - failed creating deployment [%s]", key, new.Name, e.ErrStatus.Reason)
		writeResponseError(w, &e.ErrStatus)
	case nil:
		resp.Kind = new.Kind
		resp.APIVersion = new.APIVersion
		data, err := json.Marshal(resp)
		if err != nil {
			msg := fmt.Sprintf("request was mutated but failed encoding response [%v]", err)
			glog.Infof("%s:%s - %s", key, new.Name, msg)
			writeResponseError(w, StatusInternalError(msg, new))
			return
		}
		glog.Infof("%s:%s - request mutate with success", key, new.Name)
		writeResponseCreated(w, data)
	default:
		msg := fmt.Sprintf("unknown response from server [%v, %#v]", err, resp)
		glog.Warningf("%s:%s - %s", key, new.Name, msg)
		writeResponseError(w, StatusInternalError(msg, new))
		return
	}
}

// DeploymentsOnMod mutates PUT and PATCH requests
func (h *Handler) DeploymentsOnMod(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	key := fmt.Sprintf("Req-ID=%s, Resource=deployments:%s/%s", r.Header.Get("X-Request-ID"), params["namespace"], params["deploy"])
	glog.V(3).Infof("%s, User-Agent=%s, Method=%s", key, r.Header.Get("User-Agent"), r.Method)
	switch r.Method {
	case "PATCH":
		old, errStatus := h.getDeployment(params["namespace"], params["deploy"])
		if errStatus != nil {
			glog.V(4).Infof("%s - failed retrieving deployment [%s]", key, errStatus.Message)
			writeResponseError(w, errStatus)
			return
		}
		new, err := old.DeepCopy()
		if err != nil {
			msg := fmt.Sprintf("failed deep copying obj [%v]", err)
			glog.V(3).Infof("%s -  %s", key, err)
			writeResponseError(w, StatusInternalError(msg, &v1beta1.Deployment{}))
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&new); err != nil {
			msg := fmt.Sprintf("failed decoding request body [%v]", err)
			glog.V(3).Infof("%s -  %s", key, err)
			writeResponseError(w, StatusInternalError(msg, &v1beta1.Deployment{}))
			return
		}
		defer r.Body.Close()

		// Don't let scale if the resource has a volume
		if len(old.Spec.Template.Spec.Volumes) > 0 && new.HasMultipleReplicas() {
			msg := fmt.Sprintf("found a persistent volume, unable to scale")
			glog.Infof("%s:%s - %s", key, new.Name, msg)
			writeResponseError(w, StatusInternalError(msg, new))
			return
		}

		if errStatus := h.validateContainerImage(new); errStatus != nil {
			glog.Infof("%s - %s", key, errStatus.Message)
			writeResponseError(w, errStatus)
			return
		}

		// metadata.annotations
		for _, immutableKey := range immutableAnnotations {
			// remove immutable keys, must not allow mutating the value of those keys
			delete(new.Annotations, immutableKey)
			value, exists := old.GetAnnotation(immutableKey).Value()
			if exists {
				// set the value from the old resource
				new.SetAnnotation(immutableKey, value)
			}
		}

		// Plans (metadata.labels)
		// TODO: is it removing all labels?
		var clusterPlan *platform.Plan
		clusterPlanName, fetchPlan := mustFetchClusterPlan(old, new)
		if fetchPlan {
			var errStatus *metav1.Status
			clusterPlan, errStatus = h.getPlan(clusterPlanName)
			if errStatus != nil {
				writeResponseError(w, errStatus)
				return
			}
		} else {
			// nothing was changed, maybe the request is trying to remove
			// the plan, use the old to ignore this behaviour
			new.SetClusterPlan(clusterPlanName)
		}

		storagePlan := old.GetStoragePlan()
		if storagePlan.Exists() {
			// Cannot change plan if the old resource already has one
			new.SetStoragePlan(storagePlan.String())
		} else {
			storagePlan = new.GetStoragePlan()
			// Verify if the new resource is specifying a new storage plan
			if storagePlan.Exists() {
				_, errStatus := h.getStoragePlan(storagePlan.String())
				if errStatus != nil {
					glog.V(4).Infof("%s - failed retrieving storage plan [%s]", key, errStatus.Message)
					writeResponseError(w, errStatus)
					return
				}
				// The plan exists, setup storage for the deployment (will be handled async)
				new.SetAnnotation(platform.AnnotationSetupStorage, "true")
			}
		}

		// spec.template.spec.containers
		defaultsC, newCs := v1.Container{}, new.Spec.Template.Spec.Containers
		if len(old.Spec.Template.Spec.Containers) > 0 {
			// use default values based on the old resource
			defaultsC = old.Spec.Template.Spec.Containers[0]
		}

		if len(newCs) > 0 {
			// change only allowed attributes
			defaultsC.Name = newCs[0].Name
			defaultsC.Args = newCs[0].Args
			defaultsC.Command = newCs[0].Command
			defaultsC.Env = newCs[0].Env
			defaultsC.EnvFrom = newCs[0].EnvFrom
			defaultsC.Image = newCs[0].Image
			defaultsC.Ports = newCs[0].Ports
			defaultsC.VolumeMounts = newCs[0].VolumeMounts
			if clusterPlan != nil {
				// set resources from the defined plan, if it was not
				// set rely on the value of the old resource
				defaultsC.Resources = clusterPlan.Spec.Resources
			}
		}

		// Mutate PodTemplateSpec
		new.Spec.Template = old.Spec.Template
		new.Spec.Template.Spec.Containers = []v1.Container{defaultsC}

		// Remove empty keys from map[string]string, it's required because
		// a strategic merge is decoded to an object and every directive is lost.
		// A directive for removing a key from a map[string]string is converted to
		// []byte(`{"metadata": {"labels": "key": ""}}`) and these are not removed
		// when reapplying a merge patch.
		DeleteNullKeysFromObjectMeta(&new.ObjectMeta)
		DeleteNullKeysFromObjectMeta(&new.Spec.Template.ObjectMeta)

		patch, err := StrategicMergePatch(scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion), old.GetObject(), new.GetObject())
		if err != nil {
			msg := fmt.Sprintf("failed merging patch data [%v]", err)
			glog.V(3).Infof("%s -  %s", key, err)
			writeResponseError(w, StatusInternalError(msg, &v1beta1.Deployment{}))
			return
		}

		glog.V(4).Infof("%s, DIFF: %s", key, string(patch))
		resp, err := h.usrClientset.Extensions().Deployments(params["namespace"]).Patch(new.Name, types.StrategicMergePatchType, patch)
		switch e := err.(type) {
		case *apierrors.StatusError:
			e.ErrStatus.APIVersion = resp.APIVersion
			e.ErrStatus.Kind = "Status"
			glog.Infof("%s - failed updating namespace [%s]", key, e.ErrStatus.Reason)
			writeResponseError(w, &e.ErrStatus)
		case nil:
			resp.Kind = "Deployment"
			resp.APIVersion = v1beta1.SchemeGroupVersion.Version
			data, err := runtime.Encode(scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion), resp)
			if err != nil {
				msg := fmt.Sprintf("failed encoding response [%v]", err)
				writeResponseError(w, StatusInternalError(msg, resp))
				return
			}
			glog.Infof("%s - request mutate with success", key)
			writeResponseSuccess(w, data)
		default:
			msg := fmt.Sprintf("unknown response from server [%v, %#v]", err, resp)
			glog.Warningf("%s - %s", key, msg)
			writeResponseError(w, StatusInternalError(msg, resp))
			return
		}
	default:
		msg := fmt.Sprintf(`Method "%s" not allowed.`, r.Method)
		writeResponseError(w, StatusMethodNotAllowed(msg, &v1beta1.Deployment{}))
	}
}

func (h *Handler) getDeployment(namespace, deployName string) (*draft.Deployment, *metav1.Status) {
	obj, err := h.clientset.Extensions().Deployments(namespace).Get(deployName, metav1.GetOptions{})
	if err != nil {
		switch t := err.(type) {
		case apierrors.APIStatus:
			if t.Status().Reason == metav1.StatusReasonNotFound {
				return nil, StatusNotFound(fmt.Sprintf("deployment \"%s\" not found", deployName), &v1beta1.Deployment{})
			}
		}
		return nil, StatusInternalError(fmt.Sprintf("unknown error retrieving deployment [%v]", err), &v1beta1.Deployment{})
	}
	return draft.NewDeployment(obj), nil
}

func (h *Handler) getStoragePlan(planName string) (*platform.Plan, *metav1.Status) {
	plan, errStatus := h.getPlan(planName)
	if errStatus != nil {
		return nil, errStatus
	}
	if !plan.IsStorageType() {
		msg := fmt.Sprintf(`plan "%s" has a wrong type "%s"`, plan.Name, plan.Spec.Type)
		details := &metav1.StatusDetails{
			Name:  plan.Name,
			Group: plan.GroupVersionKind().Group,
			Causes: []metav1.StatusCause{{
				Type:    metav1.CauseTypeFieldValueNotSupported,
				Message: msg,
				Field:   fmt.Sprintf(`metadata.labels[%s]`, platform.LabelStoragePlan),
			}},
		}
		return nil, StatusConflict(msg, plan, details)
	}
	return plan, nil
}

func (h *Handler) getPlan(planName string) (*platform.Plan, *metav1.Status) {
	plan := &platform.Plan{}
	err := h.tprClient.Get().
		Namespace(platform.SystemNamespace).
		Resource("plans").
		Name(planName).
		Do().
		Into(plan)

	if err != nil {
		switch t := err.(type) {
		case apierrors.APIStatus:
			if t.Status().Reason == metav1.StatusReasonNotFound {
				return nil, StatusNotFound(fmt.Sprintf(`plan "%s" not found`, planName), &platform.Plan{})
			}
		}
		return nil, StatusInternalError(fmt.Sprintf("unknown error retrieving plan [%v]", err), &platform.Plan{})
	}
	return plan, nil
}

func (h *Handler) validateContainerImage(obj *draft.Deployment) *metav1.Status {
	if len(obj.Spec.Template.Spec.Containers) > 0 {
		hasAllowedImage := false
		for _, img := range h.allowedImages {
			if obj.Spec.Template.Spec.Containers[0].Image == img {
				hasAllowedImage = true
				break
			}
		}
		if !hasAllowedImage {
			msg := fmt.Sprintf(`the image "%s" is not allowed to run in the cluster`,
				obj.Spec.Template.Spec.Containers[0].Image)
			return StatusBadRequest(msg, &v1beta1.Deployment{}, metav1.StatusReasonBadRequest)
		}
	}
	return nil
}

// mustFetchClusterPlan verifies if the plan has changed or a new one was added,
// returns the plan name and a boolean indicating if it must be fetched from kubernetes
// if the plan doesn't need to be fetch, return the plan name from the old resource
func mustFetchClusterPlan(o, n *draft.Deployment) (string, bool) {
	old := o.GetClusterPlan()
	new := n.GetClusterPlan()

	// the resource is changing plan
	if old.Exists() && new.Exists() && old.String() != new.String() {
		return new.String(), true
	}

	// A new plan is being added
	if !old.Exists() && new.Exists() {
		return new.String(), true
	}
	return old.String(), false
}
