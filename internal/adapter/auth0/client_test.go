package auth0

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/platform"
)

func TestClientAuthorizationURLUsesPKCEAndEmailConnection(t *testing.T) {
	server := newDiscoveryServer(t)
	defer server.Close()

	client, err := NewClient(context.Background(), platform.AuthConfig{
		IssuerURL:       server.URL,
		ClientID:        "client-id",
		ClientSecret:    "client-secret",
		RedirectURL:     "https://api.example.com/auth/callback",
		EmailConnection: "email",
	}, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	redirect, err := client.AuthorizationURL(context.Background(), "state-value", "nonce-value", "challenge-value")
	if err != nil {
		t.Fatalf("AuthorizationURL() error = %v", err)
	}
	parsed, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("parsing redirect: %v", err)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"client_id":             "client-id",
		"redirect_uri":          "https://api.example.com/auth/callback",
		"response_type":         "code",
		"scope":                 "openid profile email",
		"state":                 "state-value",
		"nonce":                 "nonce-value",
		"connection":            "email",
		"code_challenge":        "challenge-value",
		"code_challenge_method": "S256",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
}

func TestClientUsesBoundedHTTPClient(t *testing.T) {
	server := newDiscoveryServer(t)
	defer server.Close()

	client, err := NewClient(context.Background(), platform.AuthConfig{IssuerURL: server.URL, ClientID: "client-id"}, nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.httpClient.Timeout <= 0 {
		t.Fatal("http client timeout must be positive")
	}
}

func newDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q,"response_types_supported":["code"],"subject_types_supported":["public"],"id_token_signing_alg_values_supported":["RS256"]}`,
			server.URL, server.URL+"/authorize", server.URL+"/oauth/token", server.URL+"/jwks.json")
	})
	return server
}
