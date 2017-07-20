package authentication

// AccessToken returns the 'access_token' key from a Token
func (t Token) AccessToken() (accessToken string) {
	switch tp := t["access_token"].(type) {
	case string:
		accessToken = tp
	}
	return accessToken
}

// ExpiresIn returns the 'expires_in' key from a Token
func (t Token) ExpiresIn() (expiresIn int) {
	switch tp := t["expires_in"].(type) {
	case int:
		expiresIn = tp
	}
	return expiresIn
}

// BearerType returns the 'token_type' key from a Token
func (t Token) BearerType() (tokenType string) {
	switch tp := t["expires_in"].(type) {
	case string:
		tokenType = tp
	}
	return tokenType
}
