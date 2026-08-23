package google

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/febry3/kailopay-be/internal/platform"
	auth "github.com/febry3/kailopay-be/internal/usecase"
	"golang.org/x/oauth2"
)

const discoveryTimeout = 10 * time.Second

// IssuerURL is Google's fixed OIDC issuer. The verifier requires the
// configured issuer to match the discovery document exactly.
const IssuerURL = "https://accounts.google.com"

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	HTTPClient   *http.Client
}

type Client struct {
	oauthConfig oauth2.Config
	verifier    *oidc.IDTokenVerifier
	httpClient  *http.Client
}

func NewClient(ctx context.Context, cfg platform.GoogleConfig, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("google client id and secret are required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: discoveryTimeout}
	} else if httpClient.Timeout <= 0 {
		copy := *httpClient
		copy.Timeout = discoveryTimeout
		httpClient = &copy
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, httpClient.Timeout)
	defer cancel()
	provider, err := oidc.NewProvider(oidc.ClientContext(discoveryCtx, httpClient), IssuerURL)
	if err != nil {
		return nil, errors.Join(auth.ErrProvider, err)
	}
	return &Client{
		oauthConfig: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier:   provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		httpClient: httpClient,
	}, nil
}

func (c *Client) AuthorizationURL(ctx context.Context, state, nonce, codeChallenge string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if state == "" || nonce == "" || codeChallenge == "" {
		return "", errors.New("authentication authorization parameters are required")
	}
	return c.oauthConfig.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), nil
}

func (c *Client) Exchange(ctx context.Context, code, codeVerifier, nonce string) (auth.Identity, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(codeVerifier) == "" || strings.TrimSpace(nonce) == "" {
		return auth.Identity{}, auth.ErrInvalidIdentity
	}
	providerContext := oidc.ClientContext(ctx, c.httpClient)
	token, err := c.oauthConfig.Exchange(providerContext, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		return auth.Identity{}, errors.Join(auth.ErrProvider, err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return auth.Identity{}, auth.ErrInvalidIdentity
	}
	idToken, err := c.verifier.Verify(providerContext, rawIDToken)
	if err != nil {
		return auth.Identity{}, auth.ErrInvalidIdentity
	}
	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Nonce         string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return auth.Identity{}, auth.ErrInvalidIdentity
	}
	if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 || strings.TrimSpace(claims.Subject) == "" {
		return auth.Identity{}, auth.ErrInvalidIdentity
	}
	displayName := strings.TrimSpace(claims.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(claims.Email)
	}
	return auth.Identity{
		Provider:      auth.ProviderGoogle,
		Subject:       claims.Subject,
		Email:         strings.TrimSpace(claims.Email),
		EmailVerified: claims.EmailVerified,
		DisplayName:   displayName,
	}, nil
}

var _ auth.Provider = (*Client)(nil)
