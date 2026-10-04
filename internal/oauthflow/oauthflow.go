// Package oauthflow drives the interactive Google OAuth loopback flow used
// to connect a Gmail account, and persists the OAuth client credentials
// (the Google Cloud "Desktop app" client ID/secret) that the flow needs.
package oauthflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"inbox-bridge/internal/config"
)

// ClientCredentials holds a Google OAuth "Desktop app" client ID/secret.
type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Environment variables that supply the OAuth client instead of client.json.
// The Claude Desktop extension (manifest.json) sets these from the Client ID
// and Client secret the user types into the extension's settings.
const (
	EnvClientID     = "INBOX_BRIDGE_CLIENT_ID"
	EnvClientSecret = "INBOX_BRIDGE_CLIENT_SECRET"
)

// credentialsFromEnv returns the client from the environment, or nil if
// either variable is empty or still an unsubstituted "${user_config...}"
// placeholder (what an extension passes when the field was left blank).
func credentialsFromEnv() *ClientCredentials {
	id := strings.TrimSpace(os.Getenv(EnvClientID))
	secret := strings.TrimSpace(os.Getenv(EnvClientSecret))
	if id == "" || secret == "" || strings.HasPrefix(id, "${") || strings.HasPrefix(secret, "${") {
		return nil
	}
	return &ClientCredentials{ClientID: id, ClientSecret: secret}
}

// LoadClientCredentials returns the OAuth client from the environment if set
// (the Claude Desktop extension), otherwise from the stored client.json (the
// setup wizard). If neither exists it returns (nil, nil) rather than an
// error.
func LoadClientCredentials() (*ClientCredentials, error) {
	if creds := credentialsFromEnv(); creds != nil {
		return creds, nil
	}

	path, err := config.ClientFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading client credentials %s: %w", path, err)
	}

	var creds ClientCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing client credentials %s: %w", path, err)
	}
	return &creds, nil
}

// SaveClientCredentials writes creds to disk, creating the config directory
// if necessary.
func SaveClientCredentials(creds *ClientCredentials) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding client credentials: %w", err)
	}

	path, err := config.ClientFile()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing client credentials %s: %w", path, err)
	}
	return nil
}

// userinfoResponse is the subset of fields we need from Google's
// oauth2/v2/userinfo endpoint.
type userinfoResponse struct {
	Email string `json:"email"`
}

// PendingAuth is an OAuth loopback flow that is listening for Google's
// redirect. URL is the consent page to send the user to; Wait blocks until
// the user finishes (or abandons) it.
type PendingAuth struct {
	URL  string
	wait func() (string, *oauth2.Token, error)
}

// Wait blocks until the user completes the consent page, it fails, or three
// minutes pass, and returns the connected email and token.
func (p *PendingAuth) Wait() (string, *oauth2.Token, error) {
	return p.wait()
}

// StartAuthFlow opens a local listener and prepares Google's consent URL,
// without printing anything or opening a browser. It never writes to stdout,
// so it is safe to call from inside the MCP server, where stdout is the
// protocol channel.
func StartAuthFlow(creds *ClientCredentials) (*PendingAuth, error) {
	ctx := context.Background()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("binding loopback listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	conf := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       config.Scopes,
		Endpoint:     google.Endpoint,
	}

	state, err := generateState()
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("generating OAuth state: %w", err)
	}

	authURL := conf.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.NotFound(w, r)
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			fmt.Fprintln(w, "Gmail was not connected. You can close this tab and try again.")
			select {
			case errCh <- fmt.Errorf("authorization error: %s", errParam):
			default:
			}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			fmt.Fprintln(w, "Gmail was not connected. You can close this tab and try again.")
			select {
			case errCh <- fmt.Errorf("no code in callback"):
			default:
			}
			return
		}
		fmt.Fprintln(w, "Gmail connected. You can close this tab and go back to Claude.")
		select {
		case codeCh <- code:
		default:
		}
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	wait := func() (string, *oauth2.Token, error) {
		defer listener.Close()

		var code string
		select {
		case code = <-codeCh:
		case err := <-errCh:
			shutdown(srv)
			return "", nil, err
		case <-time.After(3 * time.Minute):
			shutdown(srv)
			return "", nil, fmt.Errorf("timed out waiting for authorization")
		}
		shutdown(srv)

		tok, err := conf.Exchange(ctx, code)
		if err != nil {
			return "", nil, fmt.Errorf("exchanging authorization code: %w", err)
		}

		email, err := discoverEmail(ctx, conf, tok)
		if err != nil {
			return "", nil, fmt.Errorf("discovering account email: %w", err)
		}

		return email, tok, nil
	}

	return &PendingAuth{URL: authURL, wait: wait}, nil
}

// RunAuthFlow drives one full interactive OAuth loopback flow for the setup
// wizard: it starts the flow, prints the consent URL (to stderr) and tries
// to open a browser, then waits for the user. It returns the connected email
// and the resulting token.
func RunAuthFlow(creds *ClientCredentials) (string, *oauth2.Token, error) {
	pending, err := StartAuthFlow(creds)
	if err != nil {
		return "", nil, err
	}

	fmt.Fprintln(os.Stderr, "Open this URL to authorize (opening your browser automatically):")
	fmt.Fprintln(os.Stderr, pending.URL)
	OpenBrowser(pending.URL)

	return pending.Wait()
}

// generateState returns a random per-flow value used as the OAuth "state"
// parameter, so the loopback callback can reject requests that don't
// originate from the authorization URL this flow just generated.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func shutdown(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

// discoverEmail identifies which Google account was just authorized. It
// tries the lightweight oauth2/v2/userinfo endpoint first; that endpoint
// requires a userinfo/openid scope which config.Scopes does not currently
// request, so on any non-success response it falls back to the Gmail
// profile endpoint (available under gmail.readonly).
func discoverEmail(ctx context.Context, conf *oauth2.Config, tok *oauth2.Token) (string, error) {
	client := conf.Client(ctx, tok)

	if email, err := fetchUserinfoEmail(client); err == nil && email != "" {
		return email, nil
	}

	return fetchGmailProfileEmail(client)
}

func fetchUserinfoEmail(client *http.Client) (string, error) {
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", fmt.Errorf("calling userinfo endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("userinfo endpoint returned %s: %s", resp.Status, string(body))
	}

	var info userinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("parsing userinfo response: %w", err)
	}
	return info.Email, nil
}

// fetchGmailProfileEmail falls back to Gmail's own profile endpoint, which
// only requires a Gmail scope (gmail.readonly is enough) rather than an
// identity scope.
func fetchGmailProfileEmail(client *http.Client) (string, error) {
	resp, err := client.Get("https://gmail.googleapis.com/gmail/v1/users/me/profile")
	if err != nil {
		return "", fmt.Errorf("calling Gmail profile endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gmail profile endpoint returned %s: %s", resp.Status, string(body))
	}

	var profile struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", fmt.Errorf("parsing Gmail profile response: %w", err)
	}
	if profile.EmailAddress == "" {
		return "", fmt.Errorf("Gmail profile response did not include an email address")
	}
	return profile.EmailAddress, nil
}

// OpenBrowser attempts to open url in the user's default browser. Errors
// are deliberately ignored -- the URL is always shown as a fallback.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// Not "cmd /c start": cmd treats the & between query parameters as
		// a command separator and truncates the consent URL.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
