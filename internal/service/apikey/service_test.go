package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	developerEnabled bool
	created          Key
	clientID         string
	found            Key
	principal        Principal
	touched          string
	listed           []Metadata
	revoked          string
}

func (s *fakeStore) DeveloperModeEnabled(context.Context, string) (bool, error) {
	return s.developerEnabled, nil
}

func (s *fakeStore) CreateForOwner(_ context.Context, _ string, clientID, _ string, key Key) (string, error) {
	s.clientID = clientID
	key.ClientID = clientID
	s.created = key
	return clientID, nil
}

func (s *fakeStore) FindActiveByPublicID(context.Context, string) (Key, Principal, error) {
	if s.found.ID == "" {
		return Key{}, Principal{}, ErrInvalidKey
	}
	return s.found, s.principal, nil
}

func (s *fakeStore) TouchLastUsed(_ context.Context, keyID string, _ time.Time) error {
	s.touched = keyID
	return nil
}

func (s *fakeStore) ListForOwner(context.Context, string) ([]Metadata, error) {
	return s.listed, nil
}

func (s *fakeStore) RevokeForOwner(_ context.Context, _ string, keyID string, _ time.Time) error {
	s.revoked = keyID
	return nil
}

func TestCreateRequiresDeveloperMode(t *testing.T) {
	store := &fakeStore{}
	service := newTestService(t, store)

	_, err := service.Create(context.Background(), "user-1", "Default")
	if !errors.Is(err, ErrDeveloperModeRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrDeveloperModeRequired)
	}
}

func TestCreateReturnsSecretOnceAndStoresOnlyHash(t *testing.T) {
	store := &fakeStore{developerEnabled: true}
	service := newTestService(t, store)

	created, err := service.Create(context.Background(), "user-1", "Default")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(created.Plaintext, "pk_test_") {
		t.Fatalf("Plaintext = %q, want pk_test_ prefix", created.Plaintext)
	}
	if store.created.SecretHash == nil || len(store.created.SecretHash) != 32 {
		t.Fatalf("stored secret hash length = %d, want 32", len(store.created.SecretHash))
	}
	if strings.Contains(created.Plaintext, string(store.created.SecretHash)) {
		t.Fatal("stored hash unexpectedly exposes plaintext")
	}
	if store.clientID == "" || store.created.ClientID != store.clientID {
		t.Fatal("key was not attached to its generated client")
	}
}

func TestAuthenticateRejectsWrongSecretAndAcceptsValidKey(t *testing.T) {
	store := &fakeStore{developerEnabled: true}
	service := newTestService(t, store)
	created, err := service.Create(context.Background(), "user-1", "Default")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	store.found = store.created
	store.principal = Principal{ClientID: store.clientID, OwnerUserID: "user-1"}

	if _, err := service.Authenticate(context.Background(), created.Plaintext+"wrong"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Authenticate(wrong) error = %v, want %v", err, ErrInvalidKey)
	}
	principal, err := service.Authenticate(context.Background(), created.Plaintext)
	if err != nil {
		t.Fatalf("Authenticate(valid) error = %v", err)
	}
	if principal.ClientID != store.clientID || store.touched != store.created.ID {
		t.Fatalf("Authenticate(valid) = %+v, touched %q", principal, store.touched)
	}
}

func TestListAndRevokeRequireDeveloperMode(t *testing.T) {
	store := &fakeStore{}
	service := newTestService(t, store)

	if _, err := service.List(context.Background(), "user-1"); !errors.Is(err, ErrDeveloperModeRequired) {
		t.Fatalf("List() error = %v", err)
	}
	if err := service.Revoke(context.Background(), "user-1", "key-1"); !errors.Is(err, ErrDeveloperModeRequired) {
		t.Fatalf("Revoke() error = %v", err)
	}

	store.developerEnabled = true
	store.listed = []Metadata{{ID: "key-1", Prefix: "pk_test_public_...last"}}
	keys, err := service.List(context.Background(), "user-1")
	if err != nil || len(keys) != 1 {
		t.Fatalf("List() = %+v, %v", keys, err)
	}
	if err := service.Revoke(context.Background(), "user-1", "key-1"); err != nil || store.revoked != "key-1" {
		t.Fatalf("Revoke() error = %v, revoked = %q", err, store.revoked)
	}
}

func newTestService(t *testing.T, store *fakeStore) *Service {
	t.Helper()
	ids := []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}
	index := 0
	service, err := New(Dependencies{
		DeveloperMode: store,
		Creator:       store,
		Finder:        store,
		UsageRecorder: store,
		Lister:        store,
		Revoker:       store,
	}, Config{
		Pepper: []byte("0123456789abcdef0123456789abcdef"),
		Random: strings.NewReader(strings.Repeat("r", 128)),
		NewID: func() (string, error) {
			id := ids[index]
			index++
			return id, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}
