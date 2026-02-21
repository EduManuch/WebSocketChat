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
	users     map[string]User   // userID -> User
	usernames map[string]string // username -> userID
}

type User struct {
	ID       string
	name     string
	email    string
	password string
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid password")
)

func NewService(jwtSecret string, tokenTTL time.Duration, JwtSecret string) *Service {
	if JwtSecret == "" {
		log.Error("Jwt secret is empty")
		return nil
	}

	return &Service{
		storage: &Storage{
			users:     make(map[string]User),
			usernames: make(map[string]string),
		},
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
	}
}

func (s *Service) Register(username, email, password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.storage.usernames[username]; exists {
		return "", errors.New("username exists")
	}

	userID := generateUserID()

	user := User{
		ID:       userID,
		name:     username,
		email:    email,
		password: password,
	}

	s.storage.users[userID] = user
	s.storage.usernames[username] = userID
	log.Debugf("user registered: %v (ID: %v)", username, userID)

	return userID, nil
}

func (s *Service) Login(username, password string) (string, error) {
	s.mu.RLock()
	defer s.mu.Unlock()

	userID, exists := s.storage.usernames[username]
	if !exists {
		return "", ErrInvalidCredentials
	}
	if s.storage.users[userID].password != password {
		return "", ErrInvalidPassword
	}

	return userID, nil
}

func (s *Service) GenerateToken(userID, username string) (string, error) {
	claims := types.Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID,
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
