package auth

import (
	"WebSocketChat/internal/types"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	storage   *Storage
	jwtSecret []byte
	tokenTTL  time.Duration
	mu        sync.RWMutex
}

type Storage struct {
	users  map[string]User   // userID -> User
	emails map[string]string // email -> userID
}

type User struct {
	name         string
	passwordHash []byte
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrEmailExists        = errors.New("email exists")
	ErrUnauthNoToken      = errors.New("unauthorized: no token provided")
)

func NewService(jwtSecret string, tokenTTL time.Duration) *Service {
	if jwtSecret == "" {
		log.Error("Jwt secret is empty")
		return nil
	}

	return &Service{
		storage: &Storage{
			users:  make(map[string]User),
			emails: make(map[string]string),
		},
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
	}
}

func (s *Service) Register(email, password, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.storage.emails[email]; exists {
		return ErrEmailExists
	}

	userID := generateUserID()
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user := User{
		name:         username,
		passwordHash: hashedPassword,
	}

	s.storage.users[userID] = user
	s.storage.emails[email] = userID
	log.Debugf("email registered: %v (ID: %v)", email, userID)

	return nil
}

func (s *Service) Login(email, password string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, exists := s.storage.emails[email]
	if !exists {
		return ErrInvalidCredentials
	}
	err := bcrypt.CompareHashAndPassword(s.storage.users[userID].passwordHash, []byte(password))
	if err != nil {
		return ErrInvalidPassword
	}

	return nil
}

func (s *Service) GenerateToken(username string) (string, error) {
	now := time.Now()
	expiresAt := now.Add(s.tokenTTL)

	log.Debugf("Generating token for %s: now=%v, tokenTTL=%v, expiresAt=%v",
		username, now, s.tokenTTL, expiresAt)

	claims := types.Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "websocket-chat",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)

	log.Debugf("Token generated: len=%d", len(tokenString))
	return tokenString, err
}

func (s *Service) JWTMiddleware(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := s.ValidateToken(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(types.ErrorResponse{err.Error()})
			log.Errorf("Token validation error: %v", err.Error())
			return
		}
		log.Debugf("Token validated for user: %s", claims.Username)

		ctx := context.WithValue(r.Context(), "user", claims)
		next(w, r.WithContext(ctx))
	}
}

func (s *Service) ValidateToken(r *http.Request) (*types.Claims, error) {
	tokenString, err := s.extractToken(r)
	if err != nil {
		return nil, err
	}
	claims := &types.Claims{}

	_, err = jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return s.jwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("websocket-chat"),
	)

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	return claims, nil
}

func (s *Service) extractToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie("auth_token")
	if err != nil || cookie.Value == "" {
		return "", ErrUnauthNoToken
	}
	return cookie.Value, nil
}

func generateUserID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b) // Рассмотреть вариант с UUID
}
