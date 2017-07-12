package authentication

// Token it's the token representation from the auth0 authentication api
// https://auth0.com/docs/api/authentication#get-token
// It's a dynamic object since it has multiple representantions
type Token map[string]interface{}

type Identity struct {
	Connection  string `json:"connection,omitempty"`
	UserID      int    `json:"user_id,omitempty"`
	Provider    string `json:"provider,omitempty"`
	IsSocial    bool   `json:"isSocial,omitempty"`
	AccessToken string `json:"access_token"`
}

// User represents an user in auth0 management API
// https://auth0.com/docs/api/management/v2#!/Users/get_users
type User struct {
	UserID        string                 `json:"user_id,omitempty"`
	Email         string                 `json:"email,omitempty"`
	EmailVerified bool                   `json:"email_verified,omitempty"`
	Username      string                 `json:"username,omitempty"`
	PhoneNumber   string                 `json:"phone_number,omitempty"`
	PhoneVerified bool                   `json:"phone_verified,omitempty"`
	CreatedAt     string                 `json:"created_at,omitempty"`
	UpdatedAt     string                 `json:"updated_at,omitempty"`
	Identities    []Identity             `json:"identities,omitempty"`
	AppMetadata   map[string]interface{} `json:"app_metadata,omitempty"`
	UserMetadata  map[string]interface{} `json:"user_metadata,omitempty"`
	Picture       string                 `json:"picture,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Nickname      string                 `json:"nickname,omitempty"`
	Multifactor   []string               `json:"multifactor,omitempty"`
	LastIP        string                 `json:"last_ip,omitempty"`
	LastLogin     string                 `json:"last_login,omitempty"`
	LoginsCount   int                    `json:"logins_count,omitempty"`
	Blocked       bool                   `json:"blocked,omitempty"`
	GivenName     string                 `json:"given_name,omitempty"`
	FamilyName    string                 `json:"family_name,omitempty"`
}
