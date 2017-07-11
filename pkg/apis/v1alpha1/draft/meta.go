package draft

import (
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Error is the error interface implementation.
func (e ErrInvalidGitSha) Error() string {
	return fmt.Sprintf("Git sha %s was invalid", e.sha)
}

// Full returns the full git sha.
func (s SHA) Full() string { return s.full }

// Short returns the first 8 characters of the sha.
func (s SHA) Short() string { return s.short }

// NamespaceMeta is a object to extract metadata information about a kubernetes namespace,
// the platform could enforce policies based with this information. The namespace
// is created using the JWT claims from an user
type NamespaceMeta struct {
	kubernetesNamespace string
	namespace           string
	customer            string
	organization        string
	valid               bool
}

func (n NamespaceMeta) Namespace() string           { return n.namespace }
func (n NamespaceMeta) Customer() string            { return n.customer }
func (n NamespaceMeta) Organization() string        { return n.organization }
func (n NamespaceMeta) Valid() bool                 { return n.valid } // DEPRECATED in flavor of IsValid
func (n NamespaceMeta) IsValid() bool               { return n.valid }
func (n NamespaceMeta) KubernetesNamespace() string { return n.kubernetesNamespace }

// MapValue represents the value from a map[string]string
type MapValue struct {
	Val string
}

func (m MapValue) AsInt() int {
	v, _ := strconv.Atoi(m.Val)
	return v
}
func (m MapValue) String() string        { return m.Val }
func (m MapValue) AsBool() bool          { return m.Val == "true" }
func (m MapValue) Exists() bool          { return len(m.Val) > 0 }
func (m MapValue) Get() (string, bool)   { return m.String(), m.Exists() } // DEPRECATED
func (m MapValue) Value() (string, bool) { return m.String(), m.Exists() }

func (o *DraftMeta) GetKubernetesObjectMeta() *metav1.ObjectMeta { return o.objectMeta }
func (o *DraftMeta) GetLabel(key string) *MapValue {
	m := &MapValue{}
	if o.objectMeta.Labels != nil {
		m.Val = o.objectMeta.Labels[key]
	}
	return m
}

func (o *DraftMeta) GetAnnotation(key string) *MapValue {
	m := &MapValue{}
	if o.objectMeta.Annotations != nil {
		m.Val = o.objectMeta.Annotations[key]
	}
	return m
}

// SetAnnotation initializes and add a new key-value annotation
func (o *DraftMeta) SetAnnotation(key, value string) {
	if o.objectMeta.Annotations == nil {
		o.objectMeta.Annotations = map[string]string{}
	}
	o.objectMeta.Annotations[key] = value
}

// SetRef initializes and add a new key=value label
func (o *DraftMeta) SetLabel(key, value string) {
	if o.objectMeta.Labels == nil {
		o.objectMeta.Labels = map[string]string{}
	}
	o.objectMeta.Labels[key] = value
}

// GetNamespaceMetadata parses the kubernetes namespace and returns a *NamespaceMeta
func (o *DraftMeta) GetNamespaceMetadata() *NamespaceMeta {
	parts := strings.Split(o.objectMeta.Namespace, "-")
	nsMeta := &NamespaceMeta{valid: false, kubernetesNamespace: o.objectMeta.Namespace}
	if len(parts) >= 3 {
		nsMeta.valid = true
		// [namespace]-[customer]-[organization]
		nsMeta.namespace = parts[0]
		nsMeta.customer = strings.Join(parts[1:len(parts)-1], "-") // could have multiple hyphens
		nsMeta.organization = parts[len(parts)-1:][0]
	}
	return nsMeta
}
