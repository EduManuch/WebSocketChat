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
	"net/mail"
	"strings"
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
}

type Storage struct {
	users  map[string]User   // userID -> User
	emails map[string]string // email -> userID
	mu     sync.RWMutex
}

type User struct {
	name         string
	passwordHash []byte
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrEmptyEmail         = errors.New("emplty email")
	ErrEmailExists        = errors.New("email exists")
	ErrInvalidToken       = errors.New("invalid token")
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
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	normEmail, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	return s.storage.addUser(normEmail, username, hashedPassword)
}

func (s *Service) Login(email, password string) error {
	normEmail, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	user, ok := s.storage.getUserByEmail(normEmail)
	if !ok {
		return ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword(user.passwordHash, []byte(password))
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
			Audience:  jwt.ClaimStrings{"ws"},
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
			json.NewEncoder(w).Encode(types.ErrorResponse{Message: "Unauthorized"})
			log.Debugf("Token validation error: %v", err.Error())
			return
		}
		log.Debugf("Token validated for user: %s", claims.Username)

		ctx := context.WithValue(r.Context(), "user", claims)
		next(w, r.WithContext(ctx))
	}
}

func (s *Service) ValidateToken(r *http.Request) (*types.Claims, error) {
	tokenString, err := extractToken(r)
	if err != nil {
		return nil, err
	}
	claims := &types.Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("websocket-chat"),
		jwt.WithAudience("ws"),
	)

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (s *Storage) getUserByEmail(email string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, exists := s.emails[email]
	if !exists {
		return User{}, false
	}
	user, ok := s.users[userID]
	return user, ok
}

func (s *Storage) addUser(email, username string, hashedPassword []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.emails[email]; exists {
		return ErrEmailExists
	}

	userID, err := generateUserID()
	if err != nil {
		return err
	}

	user := User{
		name:         username,
		passwordHash: hashedPassword,
	}
	s.users[userID] = user
	s.emails[email] = userID

	log.Debugf("user registered ID: %v)", userID)
	return nil
}

func extractToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie("auth_token")
	if err != nil || cookie.Value == "" {
		return "", ErrUnauthNoToken
	}
	return cookie.Value, nil
}

func generateUserID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), err // Рассмотреть вариант с UUID
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("invalid email format: %w", err)
	}

	if addr.Address == "" {
		return "", ErrEmptyEmail
	}

	return addr.Address, nil
}
