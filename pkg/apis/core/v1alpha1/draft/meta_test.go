package draft

import (
	"strconv"
	"strings"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamespaceMetadata(t *testing.T) {
	testCases := []struct {
		expName string
		expNs   []string
		Err     bool
	}{
		// customer with two hyphens
		{
			expName: "foo",
			expNs:   []string{"prod", "great", "zeus", "acme"},
		},
		// customer with one hyphen
		{
			expName: "foo",
			expNs:   []string{"prod", "zeus", "acme"},
		},
		// invalid namespace
		{
			expName: "foo",
			expNs:   []string{"prod", "acme"},
			Err:     true,
		},
	}
	for _, test := range testCases {
		kd := newKubernetesDeployment(test.expName, strings.Join(test.expNs, "-"), nil, nil, nil, v1.Container{})
		draft := NewDeployment(kd)
		nsMeta := draft.GetNamespaceMetadata()
		if nsMeta.Valid() && test.Err {
			t.Errorf("ISERR: true, GOT: %v, EXPECTED: false", nsMeta.Valid())
		}

		if test.Err { // skip validating further because it's an error
			continue
		}

		expNs := test.expNs[0]
		expCustomer := strings.Join(test.expNs[1:len(test.expNs)-1], "-")
		expOrganization := test.expNs[len(test.expNs)-1:][0]

		if nsMeta.Customer() != expCustomer {
			t.Errorf("ISERR: %v, GOT: %s, EXPECTED: %s", test.Err, nsMeta.Customer(), expCustomer)
		}
		if nsMeta.Namespace() != expNs {
			t.Errorf("ISERR: %v, GOT: %s, EXPECTED: %s", test.Err, nsMeta.Namespace(), expNs)
		}
		if nsMeta.Organization() != expOrganization {
			t.Errorf("ISERR: %v, GOT: %s, EXPECTED: %s", test.Err, nsMeta.Organization(), expOrganization)
		}
		if !nsMeta.Valid() {
			t.Errorf("ISERR: %v, GOT: %v, EXPECTED: true", test.Err, nsMeta.Valid())
		}
	}
}

func TestMapValue(t *testing.T) {
	// m := &MapValue{}
	// m.value
	testCases := []struct {
		value     string
		valueType interface{}
		empty     bool
	}{
		{value: "foo", valueType: "", empty: false},
		{value: "", valueType: "", empty: true},

		{value: "10", valueType: 0, empty: false},
		{value: "", valueType: 0, empty: true},

		{value: "true", valueType: false, empty: false},
		{value: "", valueType: false, empty: true},
	}
	for _, test := range testCases {
		m := MapValue{Val: test.value}

		if m.Exists() && test.empty { // must not exists when value is empty
			t.Errorf("ISEMPTY: %v, GOT: %v, EXPECTED: false", test.empty, m.Exists())
		}
		switch tp := test.valueType.(type) {
		case string:
			val, _ := m.Value()
			if m.String() != test.value || val != test.value {
				t.Errorf("GOT: %v, EXPECTED: %v", m.String(), test.value)
			}
			if m.Exists() && test.empty { // must not exists when value is empty
				t.Errorf("ISEMPTY: %v, GOT: %v, EXPECTED: false", test.empty, m.Exists())
			}
		case int:
			val := m.AsInt()
			if val != 0 && test.empty { // if it's empty must not be different from 0
				t.Errorf("ISEMPTY: %v, GOT: %d, EXPECTED: 0", test.empty, val)
			}
		case bool:
			val := m.AsBool()
			if val && test.empty {
				t.Errorf("ISEMPTY: %v, GOT: %v, EXPECTED: false", test.empty, val)
			}
			if strconv.FormatBool(val) != test.value && !test.empty { // if it's not empty, values must match
				t.Errorf("ISEMPTY: %v, GOT: %#v, EXPECTED: %v", val, test.value, test.empty)
			}
		default:
			t.Fatalf("unexpected test value: %T, VAL: %#v", tp, m)
		}

	}

}

func TestDraftMetaMethods(t *testing.T) {
	var (
		expValueLabel = "value-label"
		expValueNote  = "value-note"
	)
	d := &DraftMeta{objectMeta: &metav1.ObjectMeta{}}
	d.SetLabel("foo-key-label", expValueLabel)
	d.SetAnnotation("foo-key-note", expValueNote)

	if d.GetAnnotation("foo-key-note").String() != expValueNote {
		t.Errorf("GOT: %#v, EXPECTED: %#v", d.GetAnnotation("foo-key-note").String(), expValueNote)
	}
	if d.GetLabel("foo-key-label").String() != expValueLabel {
		t.Errorf("GOT: %#v, EXPECTED: %#v", d.GetLabel("foo-key-label").String(), expValueLabel)
	}
}
