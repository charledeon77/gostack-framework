package socialhub

import (
	"net/http"
)

// SocialUser represents a standardized user profile returned by any OAuth provider.
type SocialUser struct {
	ID        string
	Name      string
	Email     string
	AvatarURL string
	Token     string // The access token, in case the app needs to make API calls
}

// Provider defines the contract for an OAuth2 authentication provider.
type Provider interface {
	// Redirect returns the URL where the user should be sent to authenticate.
	Redirect() string

	// UserFromCallback exchanges the authorization code for an access token
	// and retrieves the user's profile information from the provider's API.
	UserFromCallback(r *http.Request) (SocialUser, error)
}
