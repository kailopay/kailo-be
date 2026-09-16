package main

import (
	"context"
	"errors"
	"testing"
)

type avatarBucketStoreFake struct {
	err         error
	allowCreate bool
}

func (s *avatarBucketStoreFake) EnsureBucket(_ context.Context, allowCreate bool) error {
	s.allowCreate = allowCreate
	return s.err
}

func TestEnsureAvatarBucketReturnsStorageErrorForOptionalStartupHandling(t *testing.T) {
	wantErr := errors.New("storage unavailable")
	store := &avatarBucketStoreFake{err: wantErr}

	err := ensureAvatarBucket(context.Background(), store, "local")
	if !errors.Is(err, wantErr) {
		t.Fatalf("ensureAvatarBucket() error = %v, want %v", err, wantErr)
	}
	if !store.allowCreate {
		t.Fatal("ensureAvatarBucket() allowCreate = false, want true for local")
	}
}
