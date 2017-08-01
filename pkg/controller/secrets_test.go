package controller

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/pkg/api/v1"
	core "k8s.io/client-go/testing"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/util"
)

func newSecret(name, ns, token string, lastUpdate time.Time) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Labels:    map[string]string{platform.LabelSecretController: "true"},
			Annotations: map[string]string{
				platform.AnnotationSecretLastUpdated: lastUpdate.Format(time.RFC3339),
			},
		},
		Data: map[string][]byte{
			"token.jwt": bytes.NewBufferString(token).Bytes(),
		},
		Type: v1.SecretTypeOpaque,
	}
}

func newNamespace(name string) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func TestSyncSecrets(t *testing.T) {
	var (
		customer, org, jwtSecret = "coyote", "acme", "asecret"
		ns                       = fmt.Sprintf("dev-%s-%s", customer, org)
		expectedObj              = newSecret(platform.SystemSecretName, ns, "", time.Now().UTC())
	)
	tokenStr, _ := util.GenerateNewJwtToken(jwtSecret, customer, org, platform.SystemTokenType, time.Now().UTC().Add(time.Hour*1))
	expectedObj.Data["token.jwt"] = bytes.NewBufferString(tokenStr).Bytes()
	f := newFixture(t, []runtime.Object{newNamespace(ns)}, nil, nil)
	c, _ := f.newSecretController("asecret")
	f.client.AddReactor("patch", "secrets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), action.GetResource().Resource)
	})
	if err := c.syncHandler(ns); err != nil {
		t.Fatalf("got unexpected error: %v", err)
	}

	if len(f.client.Actions()) != 2 {
		t.Fatalf("unexpected length of action(s), found %d action(s)", len(f.client.Actions()))
	}

	for _, action := range f.client.Actions() {
		switch a := action.(type) {
		case core.PatchActionImpl: // no-op, simulated a not found resource
		case core.CreateActionImpl:
			s := a.GetObject().(*v1.Secret)
			if !reflect.DeepEqual(s, expectedObj) {
				t.Errorf("GOT: %#v, EXPECTED: %#v", s, expectedObj)
			}
		default:
			t.Errorf("unexpected type of action: %T, OBJ: %v", a, action)
		}
	}
}

func TestSyncSecretsWithValidLastUpdatedTime(t *testing.T) {
	var (
		customer, org = "coyote", "acme"
		ns            = fmt.Sprintf("dev-%s-%s", customer, org)
		expectedObj   = newSecret(platform.SystemSecretName, ns, "", time.Now().UTC())
	)
	f := newFixture(t, []runtime.Object{newNamespace(ns)}, nil, []runtime.Object{expectedObj})
	c, _ := f.newSecretController("")
	if err := c.syncHandler(ns); err != nil {
		t.Fatalf("got unexpected error: %v", err)
	}
	if len(f.client.Actions()) != 0 {
		t.Fatalf("unexpected length of action(s), got %d action(s), expected 0", len(f.client.Actions()))
	}
}

func TestSyncSecretsWithExpiredLastUpdatedTime(t *testing.T) {
	var (
		customer, org = "coyote", "acme"
		ns            = fmt.Sprintf("dev-%s-%s", customer, org)
		expectedObj   = newSecret(platform.SystemSecretName, ns, "", time.Now().UTC().Add(time.Minute-time.Minute*25))
	)
	f := newFixture(t, []runtime.Object{newNamespace(ns)}, nil, []runtime.Object{expectedObj})
	c, _ := f.newSecretController("")
	if err := c.syncHandler(ns); err != nil {
		t.Fatalf("got unexpected error: %v", err)
	}
	if len(f.client.Actions()) != 1 {
		t.Fatalf("unexpected length of action(s), got %d action(s), expected 1", len(f.client.Actions()))
	}
}

func TestSyncSecretsWithErrorOnPatch(t *testing.T) {
	var (
		customer, org = "coyote", "acme"
		ns            = fmt.Sprintf("dev-%s-%s", customer, org)
		expectedObj   = newSecret(platform.SystemSecretName, ns, "", time.Now().UTC().Add(time.Minute-time.Minute*25))
		expError      = fmt.Errorf("failed updating secret [bad request happened]")
	)
	f := newFixture(t, []runtime.Object{newNamespace(ns)}, nil, []runtime.Object{expectedObj})
	c, _ := f.newSecretController("")
	f.client.AddReactor("patch", "secrets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewBadRequest("bad request happened")
	})
	err := c.syncHandler(ns)
	if !reflect.DeepEqual(err, expError) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", err, expError)
	}
}

func TestSyncSecretsWithErrorOnCreate(t *testing.T) {
	var (
		customer, org = "coyote", "acme"
		ns            = fmt.Sprintf("dev-%s-%s", customer, org)
		expectedObj   = newSecret(platform.SystemSecretName, ns, "", time.Now().UTC().Add(time.Minute-time.Minute*25))
		expError      = fmt.Errorf("failed creating secret [bad request happened]")
	)
	f := newFixture(t, []runtime.Object{newNamespace(ns)}, nil, []runtime.Object{expectedObj})
	c, _ := f.newSecretController("")
	f.client.AddReactor("patch", "secrets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), action.GetResource().Resource)
	})

	f.client.PrependReactor("create", "secrets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewBadRequest("bad request happened")
	})
	err := c.syncHandler(ns)
	if !reflect.DeepEqual(err, expError) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", err, expError)
	}
}
