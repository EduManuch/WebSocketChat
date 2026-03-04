package auth_test

import (
	"WebSocketChat/internal/auth"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		password    string
		username    string
		wantErr     bool
		expectedErr error
	}{
		{"валидные данные", "test@mail.com", "password123", "Bob", false, nil},
		{"пустой email", "", "password123", "Bob", true, auth.ErrEmptyEmail},
		{"невалидный email", "not-an-email", "password123", "Bob", true, nil},
		{"короткий пароль", "user@mail.com", "123", "Bob", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", time.Second*300)

			if tt.name == "дубликат email" {
				_ = service.Register("dup@mail.com", "password123", "Bob")
				tt.email = "dup@mail.com"
			}

			err := service.Register(tt.email, tt.password, tt.username)

			if tt.wantErr {
				assert.Error(t, err, tt.name)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr, err, tt.name)
				}
			} else {
				assert.NoError(t, err, tt.name)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name          string
		email         string
		regPassword   string
		loginPassword string
		setupUser     bool
		wantErr       bool
		expectedErr   error
	}{
		{"валидные данные", "test@mail.com", "password123", "password123", true, false, nil},
		{"неверный пароль", "test@mail.com", "password123", "wrongpassword", true, true, auth.ErrInvalidPassword},
		{"пользователь не существует", "unknown@mail.com", "", "password123", false, true, auth.ErrInvalidCredentials},
		{"пустой email", "", "", "password123", false, true, auth.ErrEmptyEmail},
		{"невалидный email", "not-an-email", "", "password", false, true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", time.Second*300)

			if tt.setupUser {
				err := service.Register(tt.email, tt.regPassword, "TestUser")
				assert.NoError(t, err, "setup: registration should succeed")
			}

			err := service.Login(tt.email, tt.loginPassword)

			if tt.wantErr {
				assert.Error(t, err, tt.name)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr, tt.name)
				}
			} else {
				assert.NoError(t, err, tt.name)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	tests := []struct {
		name     string
		username string
	}{
		{"валидный username", "testuser"},
		{"пустой username", ""},
		{"username с пробелами", "user name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", time.Second*300)
			token, err := service.GenerateToken(tt.username)
			assert.NoError(t, err, tt.name)
			assert.NotEmpty(t, token, tt.name)
		})
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name          string
		setupToken    func(*auth.Service) string
		expectedUser  string
		wantErr       bool
		expectedErrIs error
	}{
		{
			name: "валидный токен",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateToken("testuser")
				return token
			},
			expectedUser: "testuser",
			wantErr:      false,
		},
		{
			name: "истёкший токен",
			setupToken: func(s *auth.Service) string {
				sExpired := auth.NewService("secret", time.Second*0)
				token, _ := sExpired.GenerateToken("testuser")
				return token
			},
			wantErr:       true,
			expectedErrIs: jwt.ErrTokenExpired,
		},
		{
			name: "неверная подпись",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateToken("testuser")
				return token + "invalid"
			},
			wantErr: true,
		},
		{
			name: "токен от другого секретного ключа",
			setupToken: func(s *auth.Service) string {
				sOther := auth.NewService("other-secret", time.Second*300)
				token, _ := sOther.GenerateToken("testuser")
				return token
			},
			wantErr: true,
		},
		{
			name: "отсутствует токен",
			setupToken: func(s *auth.Service) string {
				return ""
			},
			wantErr:       true,
			expectedErrIs: auth.ErrUnauthNoToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", time.Second*300)

			tokenString := tt.setupToken(service)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tokenString != "" {
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: tokenString})
			}

			claims, err := service.ValidateToken(req)

			if tt.wantErr {
				assert.Error(t, err, tt.name)
				if tt.expectedErrIs != nil {
					assert.ErrorIs(t, err, tt.expectedErrIs, tt.name)
				}
			} else {
				assert.NoError(t, err, tt.name)
				assert.NotNil(t, claims, tt.name)
				assert.Equal(t, tt.expectedUser, claims.Username, tt.name)
			}
		})
	}
}
