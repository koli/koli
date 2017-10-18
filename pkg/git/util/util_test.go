package util

import (
	"testing"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

func TestSystemTokenClaims(t *testing.T) {
	var (
		customer, organization = "coyote", "acme"
	)
	tokenString, err := GenerateNewJwtToken("secret", customer, organization, platform.SystemTokenType)
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
	if u.ExpiresAt > 0 {
		t.Errorf("GOT: %d, EXPECTED NON EXPIRING TOKEN: 0", u.ExpiresAt)
	}
}
