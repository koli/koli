package mutator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"

	"github.com/golang/glog"

	jwt "github.com/dgrijalva/jwt-go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/rest"
)

// Config is the daemon base configuration
type Config struct {
	Host            string `envconfig:"KUBERNETES_SERVICE_HOST" required:"true"`
	TLSInsecure     bool
	TLSServerConfig rest.TLSClientConfig
	TLSClientConfig rest.TLSClientConfig
	Serve           string
	AllowedImages   string
	RegistryImages  string
}

// GetServeAddress return the address to bind the server
func (c *Config) GetServeAddress() (string, bool) {
	if len(c.TLSServerConfig.CertFile) > 0 && len(c.TLSServerConfig.KeyFile) > 0 && len(c.Serve) == 0 {
		return ":8443", true
	}
	if len(c.Serve) == 0 {
		return ":8080", false
	}
	return c.Serve, false
}

// GetImages returns of allowed images with the registry as prefix
func (c *Config) GetImages() []string {
	images := []string{}
	for _, img := range strings.Split(c.AllowedImages, ",") {
		images = append(images, filepath.Join(c.RegistryImages, img))
	}
	return images
}

func forbiddenAccessMessage(u *platform.User, customer, org string) string {
	msg := fmt.Sprintf("Permission denied. The user belongs to the customer '%s' and organization '%s', ", u.Customer, u.Organization)
	msg = msg + fmt.Sprintf("but the request was sent to the customer '%s' and organization '%s'. ", customer, org)
	return msg + fmt.Sprintf("Valid values are '[name]-%s-%s'", u.Customer, u.Organization)
}

func writeResponseCreated(w http.ResponseWriter, data []byte) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

func writeResponseSuccess(w http.ResponseWriter, data []byte) {
	w.Header().Add("Content-Type", "application/json")
	w.Write(data)
}

func writeResponseNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func writeResponseError(w http.ResponseWriter, status *metav1.Status) {
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

// decodeJwtToken decodes a jwt token into an UserMeta struct
func decodeJwtToken(header http.Header) (*platform.User, string, error) {
	// [0] = "bearer" / [1] = "<token>"{
	authorization := strings.Split(header.Get("Authorization"), " ")
	if len(authorization) != 2 {
		return nil, "", fmt.Errorf("missing token or bearer in Authorization")
	}
	parts := strings.Split(authorization[1], ".")
	if len(parts) != 3 {
		return nil, "", fmt.Errorf("it's not a valid jwt token")
	}
	// Don't care about validating tokens, only about the token data.
	seg, err := jwt.DecodeSegment(parts[1])
	if err != nil {
		return nil, "", fmt.Errorf("failed decoding segment: %s", err)
	}
	user := &platform.User{}
	return user, authorization[1], json.Unmarshal(seg, user)
}

// getKubernetesUserClients returns clients to interact with the api server
func getKubernetesUserClients(mutatorCfg *Config, bearerToken string) (*kubernetes.Clientset, rest.Interface, error) {
	var config *rest.Config
	var err error
	if mutatorCfg == nil || len(mutatorCfg.Host) == 0 {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, nil, err
		}
	} else {
		config = &rest.Config{Host: mutatorCfg.Host}
		config.TLSClientConfig = mutatorCfg.TLSClientConfig
		config.Insecure = mutatorCfg.TLSInsecure
	}
	config.BearerToken = bearerToken

	var tprConfig *rest.Config
	tprConfig = config
	tprConfig.APIPath = "/apis"
	tprConfig.GroupVersion = &platform.SchemeGroupVersion
	tprConfig.ContentType = runtime.ContentTypeJSON
	tprConfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	metav1.AddToGroupVersion(api.Scheme, platform.SchemeGroupVersion)
	platform.SchemeBuilder.AddToScheme(api.Scheme)

	userTprClient, err := rest.RESTClientFor(tprConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed retrieving user k8s tpr client from config [%v]", err)
	}
	userKubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed retrieving user k8s client from config [%v]", err)
	}
	return userKubeClient, userTprClient, nil
}

func initializeMetadata(o *metav1.ObjectMeta) {
	if o.Labels == nil {
		o.Labels = make(map[string]string)
	}
	if o.Annotations == nil {
		o.Annotations = make(map[string]string)
	}
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
	return strategicpatch.StrategicMergePatch(originalObjData, currentPatch, new)
}
