package util

import (
	"testing"
	"time"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

func TestSystemTokenClaims(t *testing.T) {
	var (
		customer, organization = "coyote", "acme"
		exp                    = time.Now().UTC().Add(time.Hour * 1)
	)
	tokenString, err := GenerateNewJwtToken(
		"secret",
		customer,
		organization,
		platform.SystemTokenType,
		time.Now().UTC().Add(time.Hour*1),
	)
	if err != nil {
		t.Errorf("unexpected error generating system token: %v", err)
	}
	u, err := DecodeUserToken(tokenString, "secret", nil)
	if err != nil {
		t.Errorf("unexpected error decoding token: %v", err)
	}
	if u.Type != platform.SystemTokenType {
		t.Errorf("GOT: %#v, EXPECTED: %s", u, platform.SystemTokenType)
	}
	if u.Customer != customer {
		t.Errorf("GOT: %#v, EXPECTED: %s", u, customer)
	}
	if u.Organization != organization {
		t.Errorf("GOT: %#v, EXPECTED: %s", u, organization)
	}
	if u.ExpiresAt != exp.Unix() {
		t.Errorf("GOT: %d, EXPECTED: %v", u.ExpiresAt, exp.Unix())
	}
}
