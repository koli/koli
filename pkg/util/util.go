package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"io"
	"net/http"
	"strconv"
	"time"

	"io/ioutil"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

// Decoder known how to decode a io.Reader into a Kubernetes resource
type Decoder struct {
	r   io.Reader
	dec runtime.Decoder
}

// NewDecoder creates a new Decoder
func NewDecoder(r io.Reader, dec runtime.Decoder) *Decoder {
	return &Decoder{r: r, dec: dec}
}

// Decode it's a helper function to decode a []byte into a known kubernetes
// resource
func (d *Decoder) Decode(v runtime.Object) error {
	data, err := ioutil.ReadAll(d.r)
	if err != nil {
		return err
	}
	// runtime
	return runtime.DecodeInto(d.dec, data, v)
}

// GenerateNewJwtToken creates a new user token to allow machine-to-machine interaction
func GenerateNewJwtToken(key, customer, org string, tokenType platform.TokenType, exp time.Time) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := make(jwt.MapClaims)
	// Set some claims
	claims["customer"] = customer
	claims["org"] = org
	claims["kolihub.io/type"] = tokenType

	// always convert to UTC time
	claims["exp"] = exp.UTC().Unix() // claims["exp"] = time.Now().UTC().Add(time.Minute * 20).Unix()
	claims["iat"] = time.Now().UTC().Unix()
	token.Claims = claims

	// Sign and get the complete encoded token as a string
	return token.SignedString(bytes.NewBufferString(key).Bytes())
}

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
	return generateStatus(msg, http.StatusUnauthorized, reason, nil)
}

// StatusInternalError returns a *metav1.Status with 500 status code
func StatusInternalError(msg string, obj runtime.Object) *metav1.Status {
	return generateStatus(msg, http.StatusInternalServerError, metav1.StatusReasonUnknown, nil)
}

// StatusBadRequest returns a *metav1.Status with 400 status code
func StatusBadRequest(msg string, obj runtime.Object, reason metav1.StatusReason) *metav1.Status {
	return generateStatus(msg, http.StatusBadRequest, reason, nil)
}

// StatusNotFound returns a *metav1.Status with 404 status code
func StatusNotFound(msg string, obj runtime.Object) *metav1.Status {
	return generateStatus(msg, http.StatusNotFound, metav1.StatusReasonNotFound, nil)
}

// StatusConflict returns a *metav1.Status with 409 status code
func StatusConflict(msg string, obj runtime.Object, details *metav1.StatusDetails) *metav1.Status {
	return generateStatus(msg, http.StatusConflict, metav1.StatusReasonConflict, details)
}

// StatusUnprocessableEntity returns a *metav1.Status with 422 status code
func StatusUnprocessableEntity(msg string, obj runtime.Object, details *metav1.StatusDetails) *metav1.Status {
	return generateStatus(msg, http.StatusUnprocessableEntity, metav1.StatusReasonInvalid, details)
}

// StatusMethodNotAllowed returns a *metav1.Status with 405 status code
func StatusMethodNotAllowed(msg string, obj runtime.Object) *metav1.Status {
	return generateStatus(msg, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed, nil)
}

// StatusForbidden returns a *metav1.Status with 403 status code
func StatusForbidden(msg string, obj runtime.Object, reason metav1.StatusReason) *metav1.Status {
	return generateStatus(msg, http.StatusForbidden, reason, nil)
}

func generateStatus(msg string, statusCode int32, reason metav1.StatusReason, details *metav1.StatusDetails) *metav1.Status {
	status := &metav1.Status{
		Code:    statusCode,
		Status:  metav1.StatusFailure,
		Message: msg,
		Reason:  reason,
		Details: details,
	}
	status.Kind = "Status"
	status.APIVersion = metav1.SchemeGroupVersion.Version
	if status.Details == nil {
		status.Details = &metav1.StatusDetails{}
	}
	return status
}

// GenAdler32Hash generates a adler32 hash from a given string
func GenAdler32Hash(text string) string {
	adler32Int := adler32.Checksum([]byte(text))
	return strconv.FormatUint(uint64(adler32Int), 16)
}
