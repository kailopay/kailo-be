package repository

import (
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
)

func TestUserProfileMapsPrivateAvatarObjectKey(t *testing.T) {
	objectKey := "avatars/user-id/avatar.png"
	profile := userProfile(entity.User{
		ID:              "user-id",
		DisplayName:     "Test User",
		AvatarObjectKey: &objectKey,
	})

	if profile.AvatarObjectKey != objectKey {
		t.Fatalf("avatar object key = %q, want %q", profile.AvatarObjectKey, objectKey)
	}
}
