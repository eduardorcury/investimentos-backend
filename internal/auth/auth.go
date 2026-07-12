package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	passwordParam = "investimentos_pwd"
	tokenTTL      = time.Hour
)

// ErrInvalidCredentials is returned when the supplied password does not match
// the stored hash.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service authenticates users against a bcrypt password hash stored in SSM
// Parameter Store and issues short-lived JWTs. The password hash doubles as the
// HMAC signing secret, so no additional secret has to be provisioned.
type Service struct {
	passwordHash []byte
	jwtSecret    []byte
}

// NewService loads the bcrypt password hash from the investimentos_pwd SSM
// parameter.
func NewService(ctx context.Context, client *ssm.Client) (*Service, error) {
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(passwordParam),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load %s from SSM: %w", passwordParam, err)
	}

	hash := []byte(aws.ToString(out.Parameter.Value))
	return &Service{passwordHash: hash, jwtSecret: hash}, nil
}

// Login validates the password against the stored bcrypt hash and, on success,
// returns a JWT signed with HS256 that is valid for one hour.
func (s *Service) Login(password string) (string, error) {
	if err := bcrypt.CompareHashAndPassword(s.passwordHash, []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   "investimentos",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) validate(tokenString string) error {
	_, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	return err
}

// Middleware rejects any request that does not carry a valid bearer token.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || s.validate(token) != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
