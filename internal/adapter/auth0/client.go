package auth0

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

type Client struct {
	oauthConfig     oauth2.Config
	verifier        *oidc.IDTokenVerifier
	httpClient      *http.Client
	emailConnection string
}

func NewClient(ctx context.Context, cfg platform.AuthConfig, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: discoveryTimeout}
	} else if httpClient.Timeout <= 0 {
		copy := *httpClient
		copy.Timeout = discoveryTimeout
		httpClient = &copy
	}
	if strings.TrimSpace(cfg.IssuerURL) == "" || strings.TrimSpace(cfg.ClientID) == "" {
		return nil, errors.New("auth0 issuer and client id are required")
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, httpClient.Timeout)
	defer cancel()
	provider, err := oidc.NewProvider(oidc.ClientContext(discoveryCtx, httpClient), cfg.IssuerURL)
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
		verifier:        provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		httpClient:      httpClient,
		emailConnection: cfg.EmailConnection,
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
		oauth2.SetAuthURLParam("connection", c.emailConnection),
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
		Nickname      string `json:"nickname"`
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
		displayName = strings.TrimSpace(claims.Nickname)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(claims.Email)
	}
	return auth.Identity{
		Provider:      auth.ProviderAuth0,
		Subject:       claims.Subject,
		Email:         strings.TrimSpace(claims.Email),
		EmailVerified: claims.EmailVerified,
		DisplayName:   displayName,
	}, nil
}

var _ auth.Provider = (*Client)(nil)
