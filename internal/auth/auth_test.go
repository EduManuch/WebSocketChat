package auth_test

import (
	"WebSocketChat/internal/auth"
	"testing"
	"time"

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
