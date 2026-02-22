package auth

import (
	"WebSocketChat/internal/types"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	log "github.com/sirupsen/logrus"
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
	name     string
	password string
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrEmailExists        = errors.New("email exists")
)

func NewService(jwtSecret string, tokenTTL time.Duration, JwtSecret string) *Service {
	if JwtSecret == "" {
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
	user := User{
		name:     username,
		password: password,
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
	if s.storage.users[userID].password != password {
		return ErrInvalidPassword
	}

	return nil
}

func (s *Service) GenerateToken(username string) (string, error) {
	claims := types.Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)

	return tokenString, err
}

func generateUserID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b) // Рассмотреть вариант с UUID
}
