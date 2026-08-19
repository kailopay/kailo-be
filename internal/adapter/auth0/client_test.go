package auth0

import (
	"context"
	"encoding/json"
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

func TestClientRequestsAuth0DatabasePasswordReset(t *testing.T) {
	var received struct {
		ClientID   string `json:"client_id"`
		Email      string `json:"email"`
		Connection string `json:"connection"`
	}
	server := httptest.NewServer(nil)
	defer server.Close()
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q,"response_types_supported":["code"],"subject_types_supported":["public"],"id_token_signing_alg_values_supported":["RS256"]}`,
				server.URL, server.URL+"/authorize", server.URL+"/oauth/token", server.URL+"/jwks.json")
		case "/dbconnections/change_password":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("reset request method/content type = %s/%q", r.Method, r.Header.Get("Content-Type"))
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decoding reset request: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	})

	client, err := NewClient(context.Background(), platform.AuthConfig{
		IssuerURL:       server.URL,
		ClientID:        "client-id",
		ClientSecret:    "client-secret",
		RedirectURL:     "https://api.example.com/auth/callback",
		EmailConnection: "Username-Password-Authentication",
	}, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.RequestPasswordReset(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if received.ClientID != "client-id" || received.Email != "user@example.com" || received.Connection != "Username-Password-Authentication" {
		t.Fatalf("reset request = %#v", received)
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
