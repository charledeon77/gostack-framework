package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"

	"github.com/charledeon77/gostack/framework/socialhub"
)

// Purpose: To provide a seamless GitHub authentication driver for the SocialHub ecosystem.
//
// Philosophy: Authentication logic shouldn't bleed into business controllers.
// By abstracting GitHub's specific API endpoints into a standardized driver,
// developers can plug-and-play "Login with GitHub" with zero cognitive overhead.
//
// Architecture:
// Implements the `socialhub.Provider` contract.
// Acts as a bridge between `golang.org/x/oauth2` and the standardized `SocialUser` struct.
//
// Choice:
// We rely on the official `golang.org/x/oauth2/github` endpoint constants rather than
// hardcoding URLs, ensuring we stay aligned with GitHub's current OAuth specifications
// and security practices.
//
// Implementation details:
// - Extracts the authorization code from the `http.Request`.
// - Exchanges the code for an Access Token securely via HTTP POST.
// - Performs a subsequent authenticated GET request to `https://api.github.com/user`
//   to map the proprietary JSON response into our unified `SocialUser` shape.

type githubDriver struct {
	config *oauth2.Config
}

// NewGithubDriver initializes a new GitHub OAuth2 provider.
func NewGithubDriver(clientID, clientSecret, redirectURL string) socialhub.Provider {
	return &githubDriver{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		},
	}
}

func (g *githubDriver) Redirect() string {
	// In a production environment, you should generate a random state string
	// and store it in the user's session to verify in the callback.
	// For simplicity in this architecture, we use a static string.
	return g.config.AuthCodeURL("gostack_state", oauth2.AccessTypeOffline)
}

func (g *githubDriver) UserFromCallback(r *http.Request) (socialhub.SocialUser, error) {
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

	// Fetch user details from GitHub API
	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: failed to get user info: %v", err)
	}
	defer resp.Body.Close()

	var ghUser struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return socialhub.SocialUser{}, fmt.Errorf("socialhub: failed to decode user response: %v", err)
	}

	// Fallback to login if name is empty
	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	return socialhub.SocialUser{
		ID:        fmt.Sprintf("%d", ghUser.ID),
		Name:      name,
		Email:     ghUser.Email,
		AvatarURL: ghUser.AvatarURL,
		Token:     token.AccessToken,
	}, nil
}
