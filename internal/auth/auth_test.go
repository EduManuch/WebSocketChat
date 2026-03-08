package auth_test

import (
	"WebSocketChat/internal/auth"
	"WebSocketChat/internal/types"
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
		setupFunc   func(*auth.Service)
		wantErr     bool
		expectedErr error
	}{
		{"валидные данные", "test@mail.com", "password123", "Bob", nil, false, nil},
		{"пустой email", "", "password123", "Bob", nil, true, auth.ErrEmptyEmail},
		{"невалидный email", "not-an-email", "password123", "Bob", nil, true, nil},
		{"короткий пароль", "user@mail.com", "123", "Bob", nil, false, nil},
		{"дубликат email", "dup@mail.com", "password123", "Bob", func(s *auth.Service) {
			_ = s.Register("dup@mail.com", "password123", "User1")
		}, true, auth.ErrEmailExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)

			if tt.setupFunc != nil {
				tt.setupFunc(service)
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
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)

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
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)
			token, err := service.GenerateToken()
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
				token, _ := s.GenerateToken()
				return token
			},
			expectedUser: "",
			wantErr:      false,
		},
		{
			name: "истёкший токен",
			setupToken: func(s *auth.Service) string {
				sExpired := auth.NewService("secret", "refresh-secret", time.Second*0, time.Hour*24)
				token, _ := sExpired.GenerateToken()
				return token
			},
			wantErr:       true,
			expectedErrIs: jwt.ErrTokenExpired,
		},
		{
			name: "неверная подпись",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateToken()
				return token + "invalid"
			},
			wantErr: true,
		},
		{
			name: "токен от другого секретного ключа",
			setupToken: func(s *auth.Service) string {
				sOther := auth.NewService("other-secret", "refresh-secret", time.Second*300, time.Hour*24)
				token, _ := sOther.GenerateToken()
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
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)

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
			}
		})
	}
}

func TestJWTMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		setupToken     func(*auth.Service) string
		expectedStatus int
		expectNextCall bool
	}{
		{
			name: "валидный токен",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateToken()
				return token
			},
			expectedStatus: http.StatusOK,
			expectNextCall: true,
		},
		{
			name: "отсутствует токен",
			setupToken: func(s *auth.Service) string {
				return ""
			},
			expectedStatus: http.StatusUnauthorized,
			expectNextCall: false,
		},
		{
			name: "невалидный токен",
			setupToken: func(s *auth.Service) string {
				return "invalid.token.here"
			},
			expectedStatus: http.StatusUnauthorized,
			expectNextCall: false,
		},
		{
			name: "истёкший токен",
			setupToken: func(s *auth.Service) string {
				sExpired := auth.NewService("secret", "refresh-secret", time.Second*0, time.Hour*24)
				token, _ := sExpired.GenerateToken()
				return token
			},
			expectedStatus: http.StatusUnauthorized,
			expectNextCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)

			nextCalled := false
			var capturedCtxUser string

			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				claims, ok := r.Context().Value(auth.UserKey).(*types.Claims)
				if ok && claims != nil {
					capturedCtxUser = claims.Username
				}
				w.WriteHeader(http.StatusOK)
			}

			handler := service.JWTMiddleware(next)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tokenString := tt.setupToken(service)
			if tokenString != "" {
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: tokenString})
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code, tt.name)
			assert.Equal(t, tt.expectNextCall, nextCalled, tt.name)

			if tt.expectNextCall {
				assert.NotNil(t, capturedCtxUser, tt.name)
			}
		})
	}
}

func TestGenerateRefreshToken(t *testing.T) {
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
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)
			token, err := service.GenerateRefreshToken()
			assert.NoError(t, err, tt.name)
			assert.NotEmpty(t, token, tt.name)
		})
	}
}

func TestValidateRefreshToken(t *testing.T) {
	tests := []struct {
		name          string
		setupToken    func(*auth.Service) string
		expectedUser  string
		wantErr       bool
		expectedErrIs error
	}{
		{
			name: "валидный refresh токен",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateRefreshToken()
				return token
			},
			expectedUser: "",
			wantErr:      false,
		},
		{
			name: "истёкший refresh токен",
			setupToken: func(s *auth.Service) string {
				sExpired := auth.NewService("secret", "refresh-secret", time.Second*300, time.Second*0)
				token, _ := sExpired.GenerateRefreshToken()
				time.Sleep(time.Millisecond * 100) // Ждём истечения
				return token
			},
			wantErr:       true,
			expectedErrIs: jwt.ErrTokenExpired,
		},
		{
			name: "неверная подпись refresh токена",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateRefreshToken()
				return token + "invalid"
			},
			wantErr: true,
		},
		{
			name: "refresh токен от другого секретного ключа",
			setupToken: func(s *auth.Service) string {
				sOther := auth.NewService("secret", "other-refresh-secret", time.Second*300, time.Hour*24)
				token, _ := sOther.GenerateRefreshToken()
				return token
			},
			wantErr: true,
		},
		{
			name: "отсутствует refresh токен",
			setupToken: func(s *auth.Service) string {
				return ""
			},
			wantErr:       true,
			expectedErrIs: auth.ErrUnauthNoToken,
		},
		{
			name: "access токен вместо refresh (разные секреты)",
			setupToken: func(s *auth.Service) string {
				token, _ := s.GenerateToken() // Генерируем access токен
				return token
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)

			tokenString := tt.setupToken(service)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tokenString != "" {
				req.AddCookie(&http.Cookie{Name: "refresh_token", Value: tokenString})
			}

			claims, err := service.ValidateRefreshToken(req)

			if tt.wantErr {
				assert.Error(t, err, tt.name)
				if tt.expectedErrIs != nil {
					assert.ErrorIs(t, err, tt.expectedErrIs, tt.name)
				}
			} else {
				assert.NoError(t, err, tt.name)
				assert.NotNil(t, claims, tt.name)
			}
		})
	}
}
