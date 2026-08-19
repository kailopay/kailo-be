package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/febry3/kailopay-be/internal/platform"
	auth "github.com/febry3/kailopay-be/internal/usecase"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var errObjectNotFound = errors.New("object not found")

type backend interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket, region string) error
	Put(ctx context.Context, bucket string, object auth.AvatarObject) error
	Open(ctx context.Context, bucket, objectKey string) (auth.AvatarFile, error)
	Delete(ctx context.Context, bucket, objectKey string) error
}

type Store struct {
	backend backend
	bucket  string
	region  string
}

func New(cfg platform.ObjectStorageConfig) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("creating MinIO client: %w", err)
	}
	return newStore(&sdkBackend{client: client}, cfg.Bucket, cfg.Region), nil
}

func newStore(backend backend, bucket, region string) *Store {
	return &Store{backend: backend, bucket: bucket, region: region}
}

func (s *Store) EnsureBucket(ctx context.Context, allowCreate bool) error {
	exists, err := s.backend.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("checking avatar bucket: %w", err)
	}
	if exists {
		return nil
	}
	if !allowCreate {
		return errors.New("avatar bucket does not exist")
	}
	if err := s.backend.MakeBucket(ctx, s.bucket, s.region); err != nil {
		return fmt.Errorf("creating avatar bucket: %w", err)
	}
	return nil
}

func (s *Store) Put(ctx context.Context, object auth.AvatarObject) error {
	if !validAvatarObjectKey(object.Key) || object.Body == nil || object.Size <= 0 {
		return auth.ErrInvalidAvatar
	}
	if err := s.backend.Put(ctx, s.bucket, object); err != nil {
		return fmt.Errorf("putting avatar object: %w", err)
	}
	return nil
}

func (s *Store) Open(ctx context.Context, objectKey string) (auth.AvatarFile, error) {
	if !validAvatarObjectKey(objectKey) {
		return auth.AvatarFile{}, auth.ErrAvatarNotFound
	}
	file, err := s.backend.Open(ctx, s.bucket, objectKey)
	if errors.Is(err, errObjectNotFound) {
		return auth.AvatarFile{}, auth.ErrAvatarNotFound
	}
	if err != nil {
		return auth.AvatarFile{}, fmt.Errorf("getting avatar object: %w", err)
	}
	return file, nil
}

func (s *Store) Delete(ctx context.Context, objectKey string) error {
	if !validAvatarObjectKey(objectKey) {
		return auth.ErrInvalidAvatar
	}
	if err := s.backend.Delete(ctx, s.bucket, objectKey); err != nil && !errors.Is(err, errObjectNotFound) {
		return fmt.Errorf("deleting avatar object: %w", err)
	}
	return nil
}

func validAvatarObjectKey(objectKey string) bool {
	cleaned := path.Clean(objectKey)
	return len(objectKey) <= 1024 && strings.HasPrefix(objectKey, "avatars/") && cleaned == objectKey &&
		!strings.Contains(objectKey, "..") && !strings.Contains(objectKey, `\`)
}

type sdkBackend struct {
	client *minio.Client
}

func (b *sdkBackend) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return b.client.BucketExists(ctx, bucket)
}

func (b *sdkBackend) MakeBucket(ctx context.Context, bucket, region string) error {
	return b.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region})
}

func (b *sdkBackend) Put(ctx context.Context, bucket string, object auth.AvatarObject) error {
	_, err := b.client.PutObject(ctx, bucket, object.Key, object.Body, object.Size, minio.PutObjectOptions{
		ContentType: object.ContentType,
	})
	return err
}

func (b *sdkBackend) Open(ctx context.Context, bucket, objectKey string) (auth.AvatarFile, error) {
	object, err := b.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return auth.AvatarFile{}, mapObjectError(err)
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return auth.AvatarFile{}, mapObjectError(err)
	}
	return auth.AvatarFile{
		Body:        object,
		Size:        info.Size,
		ContentType: info.ContentType,
		ETag:        info.ETag,
	}, nil
}

func (b *sdkBackend) Delete(ctx context.Context, bucket, objectKey string) error {
	return mapObjectError(b.client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{}))
}

func mapObjectError(err error) error {
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NotFound":
		return errObjectNotFound
	default:
		return err
	}
}

var _ auth.AvatarStore = (*Store)(nil)
var _ io.ReadCloser = (*minio.Object)(nil)
