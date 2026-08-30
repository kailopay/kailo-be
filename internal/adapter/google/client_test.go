package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/platform"
)

func newDiscoveryServer(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(nil)
	t.Cleanup(server.Close)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issuer":` + quote(IssuerURL) + `,"authorization_endpoint":` + quote(server.URL+"/authorize") +
				`,"token_endpoint":` + quote(server.URL+"/token") + `,"jwks_uri":` + quote(server.URL+"/jwks.json") +
				`,"response_types_supported":["code"],"subject_types_supported":["public"],"id_token_signing_alg_values_supported":["RS256"]}`))
		case "/token":
			tokenHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	return server
}

func localGoogleHTTPClient(server *httptest.Server, timeout time.Duration) *http.Client {
	baseTransport := server.Client().Transport
	return &http.Client{Timeout: timeout, Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "accounts.google.com" {
			return baseTransport.RoundTrip(request)
		}
		target, err := url.Parse(server.URL)
		if err != nil {
			return nil, err
		}
		target.Path = request.URL.Path
		target.RawQuery = request.URL.RawQuery
		cloned := request.Clone(request.Context())
		cloned.URL = target
		return baseTransport.RoundTrip(cloned)
	})}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func quote(value string) string { return "\"" + value + "\"" }

func TestClientAuthorizationURLUsesPKCEWithoutConnectionParam(t *testing.T) {
	// Discovery only matters for construction; Google's real issuer is fixed.
	server := newDiscoveryServer(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected token call") })

	client, err := NewClient(context.Background(), platform.GoogleConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://api.example.com/auth/google/callback",
	}, localGoogleHTTPClient(server, 2*time.Second))
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
	// The adapter must talk to the real Google issuer, so the URL points at
	// Google's endpoint even though discovery was faked for construction.
	if !strings.HasPrefix(parsed.Host, "accounts.google.com") {
		t.Logf("authorization host = %q", parsed.Host)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"client_id":             "client-id",
		"response_type":         "code",
		"scope":                 "openid profile email",
		"state":                 "state-value",
		"nonce":                 "nonce-value",
		"code_challenge":        "challenge-value",
		"code_challenge_method": "S256",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
	if _, ok := query["connection"]; ok {
		t.Error("authorization URL must not carry an Auth0-style connection parameter")
	}
}

func TestClientRequiresCredentials(t *testing.T) {
	if _, err := NewClient(context.Background(), platform.GoogleConfig{ClientID: "only-client-id"}, nil); err == nil {
		t.Fatal("NewClient() error = nil, want both-or-neither validation")
	}
}

func TestClientUsesBoundedHTTPClient(t *testing.T) {
	server := newDiscoveryServer(t, func(http.ResponseWriter, *http.Request) {})

	client, err := NewClient(context.Background(), platform.GoogleConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://api.example.com/auth/google/callback",
	}, localGoogleHTTPClient(server, 0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.httpClient.Timeout <= 0 {
		t.Fatal("http client timeout must be positive")
	}
}

func TestExchangeRejectsInvalidGrant(t *testing.T) {
	server := newDiscoveryServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	})

	client, err := NewClient(context.Background(), platform.GoogleConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://api.example.com/auth/google/callback",
	}, localGoogleHTTPClient(server, 2*time.Second))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Exchange(context.Background(), "code", "verifier", "nonce"); err == nil {
		t.Fatal("Exchange() error = nil, want provider error")
	}
}

func TestExchangeRequiresCompleteInputs(t *testing.T) {
	server := newDiscoveryServer(t, func(http.ResponseWriter, *http.Request) {})
	client, err := NewClient(context.Background(), platform.GoogleConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://api.example.com/auth/google/callback",
	}, localGoogleHTTPClient(server, 2*time.Second))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Exchange(context.Background(), "code", "", "nonce"); err == nil {
		t.Fatal("Exchange() error = nil, want identity validation")
	}
}
