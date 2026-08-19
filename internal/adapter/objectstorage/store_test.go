package objectstorage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	auth "github.com/febry3/kailopay-be/internal/usecase"
)

type fakeBackend struct {
	bucketExists  bool
	createdBucket string
	putBucket     string
	putObject     auth.AvatarObject
	openedKey     string
	opened        auth.AvatarFile
	deletedKey    string
	err           error
}

func (b *fakeBackend) BucketExists(context.Context, string) (bool, error) {
	return b.bucketExists, b.err
}

func (b *fakeBackend) MakeBucket(_ context.Context, bucket, _ string) error {
	b.createdBucket = bucket
	return b.err
}

func (b *fakeBackend) Put(_ context.Context, bucket string, object auth.AvatarObject) error {
	b.putBucket = bucket
	b.putObject = object
	return b.err
}

func (b *fakeBackend) Open(_ context.Context, _, objectKey string) (auth.AvatarFile, error) {
	b.openedKey = objectKey
	return b.opened, b.err
}

func (b *fakeBackend) Delete(_ context.Context, _, objectKey string) error {
	b.deletedKey = objectKey
	return b.err
}

func TestStoreCreatesMissingBucketOnlyWhenAllowed(t *testing.T) {
	backend := &fakeBackend{}
	store := newStore(backend, "kailopay-profile", "us-east-1")

	if err := store.EnsureBucket(context.Background(), false); err == nil {
		t.Fatal("EnsureBucket() error = nil when creation is disabled")
	}
	if err := store.EnsureBucket(context.Background(), true); err != nil {
		t.Fatalf("EnsureBucket() error = %v", err)
	}
	if backend.createdBucket != "kailopay-profile" {
		t.Fatalf("created bucket = %q", backend.createdBucket)
	}
}

func TestStoreRejectsObjectKeysOutsideAvatarPrefix(t *testing.T) {
	backend := &fakeBackend{}
	store := newStore(backend, "kailopay-profile", "us-east-1")

	err := store.Put(context.Background(), auth.AvatarObject{
		Key:         "../private.txt",
		Body:        strings.NewReader("data"),
		Size:        4,
		ContentType: "image/png",
	})
	if err == nil {
		t.Fatal("Put() error = nil for invalid object key")
	}
	if backend.putObject.Key != "" {
		t.Fatal("invalid object reached storage backend")
	}
}

func TestStoreDelegatesPrivateAvatarOperations(t *testing.T) {
	backend := &fakeBackend{opened: auth.AvatarFile{
		Body:        io.NopCloser(strings.NewReader("avatar")),
		Size:        6,
		ContentType: "image/png",
	}}
	store := newStore(backend, "kailopay-profile", "us-east-1")
	object := auth.AvatarObject{
		Key:         "avatars/user-id/avatar.png",
		Body:        strings.NewReader("avatar"),
		Size:        6,
		ContentType: "image/png",
	}
	if err := store.Put(context.Background(), object); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	file, err := store.Open(context.Background(), object.Key)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Body.Close()
	if err := store.Delete(context.Background(), object.Key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if backend.putBucket != "kailopay-profile" || backend.openedKey != object.Key || backend.deletedKey != object.Key {
		t.Fatalf("backend calls = %q/%q/%q", backend.putBucket, backend.openedKey, backend.deletedKey)
	}
}

func TestStoreMapsMissingObjectToAvatarNotFound(t *testing.T) {
	backend := &fakeBackend{err: errObjectNotFound}
	store := newStore(backend, "kailopay-profile", "us-east-1")

	if _, err := store.Open(context.Background(), "avatars/user-id/missing.png"); !errors.Is(err, auth.ErrAvatarNotFound) {
		t.Fatalf("Open() error = %v, want %v", err, auth.ErrAvatarNotFound)
	}
}
