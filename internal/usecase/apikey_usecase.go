package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const testKeyPrefix = "pk_test_"

var (
	ErrDeveloperModeRequired = errors.New("developer mode is required")
	ErrInvalidKey            = errors.New("invalid api key")
	ErrInvalidName           = errors.New("invalid api key name")
)

type Key struct {
	ID         string
	ClientID   string
	PublicID   string
	Prefix     string
	SecretHash []byte
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

type Principal struct {
	ClientID    string
	OwnerUserID string
}

type CreatedKey struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"clientId"`
	Prefix    string    `json:"prefix"`
	Plaintext string    `json:"key"`
	CreatedAt time.Time `json:"createdAt"`
}

type Metadata struct {
	ID         string     `json:"id"`
	ClientID   string     `json:"clientId"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type DeveloperModeReader interface {
	DeveloperModeEnabled(ctx context.Context, userID string) (bool, error)
}

type Creator interface {
	CreateForOwner(ctx context.Context, ownerUserID, suggestedClientID, clientName string, key Key) (string, error)
}

type ActiveFinder interface {
	FindActiveByPublicID(ctx context.Context, publicID string) (Key, Principal, error)
}

type UsageRecorder interface {
	TouchLastUsed(ctx context.Context, keyID string, usedAt time.Time) error
}

type Lister interface {
	ListForOwner(ctx context.Context, ownerUserID string) ([]Metadata, error)
}

type Revoker interface {
	RevokeForOwner(ctx context.Context, ownerUserID, keyID string, revokedAt time.Time) error
}

type APIKeyDependencies struct {
	DeveloperMode DeveloperModeReader
	Creator       Creator
	Finder        ActiveFinder
	UsageRecorder UsageRecorder
	Lister        Lister
	Revoker       Revoker
}

type APIKeyConfig struct {
	Pepper []byte
	Random io.Reader
	NewID  func() (string, error)
	Now    func() time.Time
}

type APIKeyUsecase struct {
	dependencies APIKeyDependencies
	config       APIKeyConfig
}

func NewAPIKeyUsecase(dependencies APIKeyDependencies, config APIKeyConfig) (*APIKeyUsecase, error) {
	if dependencies.DeveloperMode == nil || dependencies.Creator == nil ||
		dependencies.Finder == nil || dependencies.UsageRecorder == nil ||
		dependencies.Lister == nil || dependencies.Revoker == nil {
		return nil, errors.New("api key dependencies are required")
	}
	if len(config.Pepper) < 32 || config.Random == nil || config.NewID == nil || config.Now == nil {
		return nil, errors.New("valid api key configuration is required")
	}
	return &APIKeyUsecase{dependencies: dependencies, config: config}, nil
}

func (s *APIKeyUsecase) List(ctx context.Context, ownerUserID string) ([]Metadata, error) {
	if err := s.requireDeveloperMode(ctx, ownerUserID); err != nil {
		return nil, err
	}
	keys, err := s.dependencies.Lister.ListForOwner(ctx, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	if keys == nil {
		keys = []Metadata{}
	}
	return keys, nil
}

func (s *APIKeyUsecase) Revoke(ctx context.Context, ownerUserID, keyID string) error {
	if strings.TrimSpace(keyID) == "" {
		return ErrInvalidKey
	}
	if err := s.requireDeveloperMode(ctx, ownerUserID); err != nil {
		return err
	}
	if err := s.dependencies.Revoker.RevokeForOwner(ctx, ownerUserID, keyID, s.config.Now().UTC()); err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}
	return nil
}

func (s *APIKeyUsecase) Create(ctx context.Context, ownerUserID, name string) (CreatedKey, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	name = strings.TrimSpace(name)
	if ownerUserID == "" || name == "" || len(name) > 100 {
		return CreatedKey{}, ErrInvalidName
	}
	enabled, err := s.dependencies.DeveloperMode.DeveloperModeEnabled(ctx, ownerUserID)
	if err != nil {
		return CreatedKey{}, fmt.Errorf("checking developer mode: %w", err)
	}
	if !enabled {
		return CreatedKey{}, ErrDeveloperModeRequired
	}

	clientID, err := s.config.NewID()
	if err != nil {
		return CreatedKey{}, fmt.Errorf("generating api client id: %w", err)
	}
	keyID, err := s.config.NewID()
	if err != nil {
		return CreatedKey{}, fmt.Errorf("generating api key id: %w", err)
	}
	publicIDBytes := make([]byte, 12)
	if _, err := io.ReadFull(s.config.Random, publicIDBytes); err != nil {
		return CreatedKey{}, fmt.Errorf("generating api key public id: %w", err)
	}
	secretBytes := make([]byte, 32)
	if _, err := io.ReadFull(s.config.Random, secretBytes); err != nil {
		return CreatedKey{}, fmt.Errorf("generating api key secret: %w", err)
	}
	publicID := hex.EncodeToString(publicIDBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	plaintext := testKeyPrefix + publicID + "_" + secret
	now := s.config.Now().UTC()
	prefix := testKeyPrefix + publicID + "_..." + secret[len(secret)-4:]
	key := Key{
		ID:         keyID,
		ClientID:   clientID,
		PublicID:   publicID,
		Prefix:     prefix,
		SecretHash: hashSecret(s.config.Pepper, secret),
		CreatedAt:  now,
	}
	clientID, err = s.dependencies.Creator.CreateForOwner(ctx, ownerUserID, clientID, name, key)
	if err != nil {
		return CreatedKey{}, fmt.Errorf("creating api key: %w", err)
	}
	key.ClientID = clientID
	return CreatedKey{ID: key.ID, ClientID: clientID, Prefix: prefix, Plaintext: plaintext, CreatedAt: now}, nil
}

func (s *APIKeyUsecase) Authenticate(ctx context.Context, rawKey string) (Principal, error) {
	publicID, secret, ok := parse(rawKey)
	if !ok {
		return Principal{}, ErrInvalidKey
	}
	key, principal, err := s.dependencies.Finder.FindActiveByPublicID(ctx, publicID)
	if err != nil {
		return Principal{}, ErrInvalidKey
	}
	providedHash := hashSecret(s.config.Pepper, secret)
	if !hmac.Equal(key.SecretHash, providedHash) {
		return Principal{}, ErrInvalidKey
	}
	if err := s.dependencies.UsageRecorder.TouchLastUsed(ctx, key.ID, s.config.Now().UTC()); err != nil {
		return Principal{}, fmt.Errorf("recording api key usage: %w", err)
	}
	return principal, nil
}

func (s *APIKeyUsecase) requireDeveloperMode(ctx context.Context, ownerUserID string) error {
	if strings.TrimSpace(ownerUserID) == "" {
		return ErrDeveloperModeRequired
	}
	enabled, err := s.dependencies.DeveloperMode.DeveloperModeEnabled(ctx, ownerUserID)
	if err != nil {
		return fmt.Errorf("checking developer mode: %w", err)
	}
	if !enabled {
		return ErrDeveloperModeRequired
	}
	return nil
}

func parse(rawKey string) (string, string, bool) {
	remainder, ok := strings.CutPrefix(strings.TrimSpace(rawKey), testKeyPrefix)
	if !ok {
		return "", "", false
	}
	publicID, secret, ok := strings.Cut(remainder, "_")
	if !ok || len(publicID) != 24 || secret == "" {
		return "", "", false
	}
	if _, err := hex.DecodeString(publicID); err != nil {
		return "", "", false
	}
	return publicID, secret, true
}

func hashSecret(pepper []byte, secret string) []byte {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(secret))
	return mac.Sum(nil)
}
