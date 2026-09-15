package repository

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestIsUniqueViolationRecognizesGORMTranslatedDuplicateKey(t *testing.T) {
	if !isUniqueViolation(gorm.ErrDuplicatedKey) {
		t.Fatalf("isUniqueViolation(%v) = false, want true", gorm.ErrDuplicatedKey)
	}

	if isUniqueViolation(errors.New("database connection failed")) {
		t.Fatal("isUniqueViolation() = true for a non-duplicate error")
	}
}
