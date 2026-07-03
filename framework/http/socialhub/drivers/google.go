package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/charledeon77/gostack-framework/framework/http/socialhub"
)

// Purpose: To provide a seamless Google authentication driver for the SocialHub ecosystem.
//
// Philosophy: Authentication logic shouldn't bleed into business controllers.
// By abstracting Google's specific API endpoints into a standardized driver,
// developers can plug-and-play "Login with Google" with zero cognitive overhead.
//
// Architecture:
// Implements the `socialhub.Provider` contract.
// Acts as a bridge between `golang.org/x/oauth2` and the standardized `SocialUser` struct.
//
// Choice:
// We rely on the official `golang.org/x/oauth2/google` endpoint constants rather than
// hardcoding URLs, ensuring we stay aligned with Google's OAuth specifications.
// We specifically request the "userinfo.profile" and "userinfo.email" scopes
// to extract the avatar and email without over-requesting permissions.
//
// Implementation details:
// - Extracts the authorization code from the `http.Request`.
// - Exchanges the code for an Access Token securely via HTTP POST.
// - Performs a subsequent authenticated GET request to `https://www.googleapis.com/oauth2/v2/userinfo`
//   to map the proprietary JSON response into our unified `SocialUser` shape.

type googleDriver struct {
	config *oauth2.Config
}

// NewGoogleDriver initializes a new Google OAuth2 provider.
func NewGoogleDriver(clientID, clientSecret, redirectURL string) socialhub.Provider {
	return &googleDriver{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.profile",
				"https://www.googleapis.com/auth/userinfo.email",
			},
			Endpoint: google.Endpoint,
		},
	}
}

func (g *googleDriver) Redirect() string {
	return g.config.AuthCodeURL("gostack_state", oauth2.AccessTypeOffline)
}

func (g *googleDriver) UserFromCallback(r *http.Request) (socialhub.SocialUser, error) {
	state := r.URL.Query().Get("state")
	if state != "gostack_state" {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: invalid oauth state")
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: code not found in request")
	}

	ctx := context.Background()
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: failed to exchange token: %v", err)
	}

	// Fetch user details from Google API
	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: failed to get user info: %v", err)
	}
	defer resp.Body.Close()

	var googUser struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&googUser); err != nil {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: failed to decode user response: %v", err)
	}

	return socialhub.SocialUser{
		ID:        googUser.ID,
		Name:      googUser.Name,
		Email:     googUser.Email,
		AvatarURL: googUser.Picture,
		Token:     token.AccessToken,
	}, nil
}
