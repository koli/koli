package util

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
)

// StrategicMergePatch creates a strategic merge patch and merge with the original object
// https://github.com/kubernetes/community/blob/master/contributors/devel/strategic-merge-patch.md
func StrategicMergePatch(codec runtime.Codec, original, new runtime.Object) ([]byte, error) {
	originalObjData, err := runtime.Encode(codec, original)
	if err != nil {
		return nil, fmt.Errorf("failed encoding original object: %v", err)
	}
	newObjData, err := runtime.Encode(codec, new)
	if err != nil {
		return nil, fmt.Errorf("failed encoding new object: %v", err)
	}
	currentPatch, err := strategicpatch.CreateTwoWayMergePatch(originalObjData, newObjData, new)
	if err != nil {
		return nil, fmt.Errorf("failed creating two way merge patch: %v", err)
	}
	return currentPatch, nil
	// return strategicpatch.StrategicMergePatch(originalObjData, currentPatch, new)
}

// DeleteNullKeysFromObjectMeta will remove any key with an empty string in .metadata.labels
// and .metadata.annotations
func DeleteNullKeysFromObjectMeta(obj *metav1.ObjectMeta) {
	for key, value := range obj.Labels {
		if len(value) == 0 {
			delete(obj.Labels, key)
		}
	}
	for key, value := range obj.Annotations {
		if len(value) == 0 {
			delete(obj.Annotations, key)
		}
	}
}

func WriteResponseCreated(w http.ResponseWriter, data []byte) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

func WriteResponseSuccess(w http.ResponseWriter, data []byte) {
	w.Header().Add("Content-Type", "application/json")
	w.Write(data)
}

func WriteResponseNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func WriteResponseError(w http.ResponseWriter, status *metav1.Status) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(int(status.Code))
	if err := json.NewEncoder(w).Encode(status); err != nil {
		glog.Infof(`{"message": "error encoding response: %s"}`, err)
		fmt.Fprintf(w, "error encoding response\n")
	}
}

// StatusUnauthorized returns a *metav1.Status with 401 status code
func StatusUnauthorized(msg string, obj runtime.Object, reason metav1.StatusReason) *metav1.Status {
	return generateStatus(msg, obj, http.StatusUnauthorized, reason, nil)
}

// StatusInternalError returns a *metav1.Status with 500 status code
func StatusInternalError(msg string, obj runtime.Object) *metav1.Status {
	return generateStatus(msg, obj, http.StatusInternalServerError, metav1.StatusReasonUnknown, nil)
}

// StatusBadRequest returns a *metav1.Status with 400 status code
func StatusBadRequest(msg string, obj runtime.Object, reason metav1.StatusReason) *metav1.Status {
	return generateStatus(msg, obj, http.StatusBadRequest, reason, nil)
}

// StatusNotFound returns a *metav1.Status with 404 status code
func StatusNotFound(msg string, obj runtime.Object) *metav1.Status {
	return generateStatus(msg, obj, http.StatusNotFound, metav1.StatusReasonNotFound, nil)
}

// StatusConflict returns a *metav1.Status with 409 status code
func StatusConflict(msg string, obj runtime.Object, details *metav1.StatusDetails) *metav1.Status {
	return generateStatus(msg, obj, http.StatusConflict, metav1.StatusReasonConflict, details)
}

// StatusUnprocessableEntity returns a *metav1.Status with 422 status code
func StatusUnprocessableEntity(msg string, obj runtime.Object, details *metav1.StatusDetails) *metav1.Status {
	return generateStatus(msg, obj, http.StatusUnprocessableEntity, metav1.StatusReasonInvalid, details)
}

// StatusMethodNotAllowed returns a *metav1.Status with 405 status code
func StatusMethodNotAllowed(msg string, obj runtime.Object) *metav1.Status {
	return generateStatus(msg, obj, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed, nil)
}

// StatusForbidden returns a *metav1.Status with 403 status code
func StatusForbidden(msg string, obj runtime.Object, reason metav1.StatusReason) *metav1.Status {
	return generateStatus(msg, obj, http.StatusForbidden, reason, nil)
}

func generateStatus(msg string, obj runtime.Object, statusCode int32, reason metav1.StatusReason, details *metav1.StatusDetails) *metav1.Status {
	if obj == nil {
		obj = &metav1.TypeMeta{APIVersion: platform.GroupName}
	}
	gvk := obj.GetObjectKind().GroupVersionKind()
	status := &metav1.Status{
		Code:    statusCode,
		Status:  metav1.StatusFailure,
		Message: msg,
		Reason:  reason,
		Details: details,
	}
	status.Kind = "Status"
	status.APIVersion = gvk.Version
	if status.Details == nil {
		status.Details = &metav1.StatusDetails{}
	}
	return status
}
