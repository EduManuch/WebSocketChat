package auth

import (
	"WebSocketChat/internal/types"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	storage          *Storage
	jwtSecret        []byte
	refreshJwtSecret []byte
	tokenTTL         time.Duration
	refreshTokenTTL  time.Duration
	fakeHash         []byte // защита от timing attack
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

type ContextKey string

const UserKey ContextKey = "user"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrEmptyEmail         = errors.New("empty email")
	ErrEmailExists        = errors.New("email exists")
	ErrInvalidToken       = errors.New("invalid token")
	ErrUnauthNoToken      = errors.New("unauthorized: no token provided")
)

func NewService(jwtSecret, refreshJwtSecret string, tokenTTL, refreshTokenTTL time.Duration) *Service {
	if jwtSecret == "" {
		log.Error("Jwt secret is empty")
		return nil
	}

	// защита от timing attack
	fakeHash, err := bcrypt.GenerateFromPassword([]byte("random-fake-hash-asd3ec34ee"), bcrypt.DefaultCost)
	if err != nil {
		log.Error("error generating hash")
		return nil
	}

	return &Service{
		storage: &Storage{
			users:  make(map[string]User),
			emails: make(map[string]string),
		},
		jwtSecret:        []byte(jwtSecret),
		refreshJwtSecret: []byte(refreshJwtSecret),
		tokenTTL:         tokenTTL,
		refreshTokenTTL:  refreshTokenTTL,
		fakeHash:         fakeHash,
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
		// защита от timing attack
		_ = bcrypt.CompareHashAndPassword(s.fakeHash, []byte("any"))
		return ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword(user.passwordHash, []byte(password))
	if err != nil {
		return ErrInvalidPassword
	}

	return nil
}

func (s *Service) JWTMiddleware(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := s.ValidateToken(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(types.ErrorResponse{Message: "Unauthorized"})
			log.Debugf("Token validation error: %v", err.Error())
			return
		}
		log.Debug("Token validated")

		ctx := context.WithValue(r.Context(), UserKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func (s *Service) GenerateToken() (string, error) {
	now := time.Now()
	expiresAt := now.Add(s.tokenTTL)

	claims := types.Claims{
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

func (s *Service) ValidateToken(r *http.Request) (*types.Claims, error) {
	tokenString, err := extractToken(r, "auth_token")
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

func (s *Service) GenerateRefreshToken() (string, error) {
	now := time.Now()
	expiresAt := now.Add(s.refreshTokenTTL)

	claims := types.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "websocket-chat",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.refreshJwtSecret)

	log.Debugf("Token generated: len=%d", len(tokenString))
	return tokenString, err
}

func (s *Service) ValidateRefreshToken(r *http.Request) (*types.Claims, error) {
	tokenString, err := extractToken(r, "refresh_token")
	if err != nil {
		return nil, err
	}
	claims := &types.Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.refreshJwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("websocket-chat"),
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

func extractToken(r *http.Request, typeToken string) (string, error) {
	cookie, err := r.Cookie(typeToken)
	if err != nil || cookie.Value == "" {
		return "", ErrUnauthNoToken
	}
	return cookie.Value, nil
}

func generateUserID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func normalizeEmail(email string) (string, error) {
	if email == "" {
		return "", ErrEmptyEmail
	}

	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("invalid email format: %w", err)
	}

	return addr.Address, nil
}
