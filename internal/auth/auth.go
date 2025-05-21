package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 0)
	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}

func CheckPasswordHash(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return err
	}
	return nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	// Create token
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	})

	// Sign token with tokenSecret key
	s, err := tok.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}

	return s, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	// Claims struct to populate
	var myClaims jwt.RegisteredClaims

	// Validate token
	_, err := jwt.ParseWithClaims(tokenString, &myClaims, func(t *jwt.Token) (interface{}, error) {
		return interface{}([]byte(tokenSecret)), nil
	})

	// Check for error
	if err != nil {
		return uuid.Nil, err
	}

	// If here, then successful
	// Parse uuid string to UUID type
	userId, err := uuid.Parse(myClaims.Subject)
	if err != nil {
		return uuid.Nil, err
	}

	// Return user ID, nil
	return userId, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	s := headers.Get("Authorization")
	if s == "" {
		return "", errors.New("no Authorization header")
	}

	// Strip 'Bearer '
	s = strings.TrimPrefix(s, "Bearer ")

	return s, nil
}

func MakeRefreshToken() (string, error) {
	// Initialize 256 byte slice
	tokenBytes := make([]byte, 32)

	// Random data fead into tokenBytes
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}

	// Convert bytes to hex string
	hexString := hex.EncodeToString(tokenBytes)

	return hexString, nil
}
