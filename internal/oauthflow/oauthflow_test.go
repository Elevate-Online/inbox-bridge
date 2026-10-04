package oauthflow

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCredentialsFromEnv(t *testing.T) {
	cases := []struct {
		name, id, secret string
		want             bool
	}{
		{"both set", "id.apps.googleusercontent.com", "secret", true},
		{"id missing", "", "secret", false},
		{"secret missing", "id", "", false},
		{"unsubstituted placeholder", "${user_config.client_id}", "${user_config.client_secret}", false},
		{"whitespace only", "  ", " ", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvClientID, c.id)
			t.Setenv(EnvClientSecret, c.secret)
			got := credentialsFromEnv()
			if (got != nil) != c.want {
				t.Fatalf("credentialsFromEnv() = %v, want present=%v", got, c.want)
			}
		})
	}
}

func TestLoadClientCredentialsPrefersEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv(EnvClientID, "env-id")
	t.Setenv(EnvClientSecret, "env-secret")

	creds, err := LoadClientCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil || creds.ClientID != "env-id" || creds.ClientSecret != "env-secret" {
		t.Fatalf("got %+v, want the env credentials", creds)
	}
}

// startTestFlow starts a flow with a fake client and returns it with its
// loopback redirect URL and state, read back out of the consent URL.
func startTestFlow(t *testing.T) (*PendingAuth, string, string) {
	t.Helper()
	pending, err := StartAuthFlow(&ClientCredentials{ClientID: "fake", ClientSecret: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(pending.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:") {
		t.Fatalf("redirect_uri = %q, want a loopback address", q.Get("redirect_uri"))
	}
	if q.Get("access_type") != "offline" {
		t.Fatalf("access_type = %q, want offline so a refresh token is issued", q.Get("access_type"))
	}
	return pending, q.Get("redirect_uri"), q.Get("state")
}

func TestCallbackRejectsWrongState(t *testing.T) {
	pending, redirect, _ := startTestFlow(t)

	resp, err := http.Get(redirect + "/?state=wrong&code=abc")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a mismatched state", resp.StatusCode)
	}

	// Finish the flow so its listener shuts down.
	resp, err = http.Get(redirect + "/?state=" + url.QueryEscape(stateOf(t, pending)) + "&error=access_denied")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, _, err := pending.Wait(); err == nil {
		t.Fatal("Wait() succeeded after access_denied, want an error")
	}
}

func TestCallbackReportsDenial(t *testing.T) {
	pending, redirect, state := startTestFlow(t)

	resp, err := http.Get(redirect + "/?state=" + url.QueryEscape(state) + "&error=access_denied")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	_, _, err = pending.Wait()
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("Wait() error = %v, want one mentioning access_denied", err)
	}
}

func stateOf(t *testing.T, p *PendingAuth) string {
	t.Helper()
	u, err := url.Parse(p.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("state")
}
