package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeJWT(t *testing.T) {
	// Tests
	userId := uuid.New()
	tokenSecret := "testingtesting"
	expiresIn := time.Hour

	// Create token, check for unexpected error
	tok, err := MakeJWT(userId, tokenSecret, expiresIn)
	if err != nil {
		t.Errorf("MakeJWT returned an unexpected error: %s", err)
		t.Fail()
	}

	// Check if the returned token string is not empty
	if tok == "" {
		t.Error("MakeJWT returned an empty token string")
		t.Fail()
	}

	// Validate string
	returnedId, err := ValidateJWT(tok, tokenSecret)
	if err != nil {
		t.Errorf("ValidateJWT returned an unecpected error: %s", err)
		t.Fail()
	}

	// Assert returnedId = userId
	if returnedId != userId {
		t.Errorf("ValidateJWT returned %v when test case was %v", returnedId, userId)
		t.Fail()
	}
}
