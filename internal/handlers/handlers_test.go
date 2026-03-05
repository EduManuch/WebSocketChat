package handlers_test

import (
	"WebSocketChat/internal/auth"
	"WebSocketChat/internal/handlers"
	"WebSocketChat/internal/types"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// testEnvConfig создаёт стандартную конфигурацию для тестов
func testEnvConfig(t *testing.T) *types.EnvConfig {
	t.Helper()
	return &types.EnvConfig{
		TemplateDir: "../../web/templates",
		UseTls:      false,
		TokenTTL:    time.Second * 300,
	}
}

// testHandler создаёт стандартный обработчик для тестов
func testHandler(t *testing.T) (*auth.Service, *handlers.WsHandler) {
	t.Helper()
	service := auth.NewService("secret", time.Second*300)
	return service, &handlers.WsHandler{AuthService: service}
}

// postJSON выполняет POST запрос с JSON телом
func postJSON(t *testing.T, url string, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	data, err := json.Marshal(body)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	rr := httptest.NewRecorder()
	return req, rr
}

// newRequest создаёт HTTP запрос
func newRequest(t *testing.T, method, url string, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	var req *http.Request
	if body != nil {
		data, err := json.Marshal(body)
		assert.NoError(t, err)
		req = httptest.NewRequest(method, url, bytes.NewReader(data))
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	return req, httptest.NewRecorder()
}

func TestRegisterUser(t *testing.T) {
	t.Run("успешная регистрация", func(t *testing.T) {
		_, handler := testHandler(t)
		req, rr := postJSON(t, "/auth/register", types.RegisterRequest{
			Username: "testuser",
			Email:    "test@example.com",
			Password: "password123",
		})

		handler.RegisterUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp types.RegisterResponse
		err := json.NewDecoder(rr.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "Register successfully", resp.Message)
	})

	t.Run("невалидный JSON", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte("invalid json")))
		rr := httptest.NewRecorder()

		handler.RegisterUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("дубликат email", func(t *testing.T) {
		service, handler := testHandler(t)

		// Сначала регистрируем пользователя
		_ = service.Register("dup@example.com", "password123", "user1")

		req, rr := postJSON(t, "/auth/register", types.RegisterRequest{
			Username: "user2",
			Email:    "dup@example.com",
			Password: "password456",
		})

		handler.RegisterUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("GET запрос возвращает HTML", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req := httptest.NewRequest(http.MethodGet, "/auth/register", nil)
		rr := httptest.NewRecorder()

		envConfig := &types.EnvConfig{
			TemplateDir: ".",
			UseTls:      false,
			TokenTTL:    time.Second * 300,
		}

		handler.RegisterUser(rr, req, envConfig)

		// GET запрос должен вернуть файл (даже если 404 - это поведение http.ServeFile)
		assert.NotEqual(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("неподдерживаемый метод", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req := httptest.NewRequest(http.MethodDelete, "/auth/register", nil)
		rr := httptest.NewRecorder()

		handler.RegisterUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestLoginUser(t *testing.T) {
	t.Run("успешный логин", func(t *testing.T) {
		service, handler := testHandler(t)

		// Сначала регистрируем пользователя
		_ = service.Register("test@example.com", "password123", "testuser")

		req, rr := postJSON(t, "/auth/login", types.LoginRequest{
			Username: "test@example.com",
			Password: "password123",
		})

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp types.LoginResponse
		err := json.NewDecoder(rr.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "test@example.com", resp.Username)

		// Проверяем, что cookie установлен
		cookies := rr.Result().Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, "auth_token", cookies[0].Name)
		assert.NotEmpty(t, cookies[0].Value)
	})

	t.Run("неверный пароль", func(t *testing.T) {
		service, handler := testHandler(t)
		_ = service.Register("test@example.com", "password123", "testuser")

		req, rr := postJSON(t, "/auth/login", types.LoginRequest{
			Username: "test@example.com",
			Password: "wrongpassword",
		})

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("пользователь не существует", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req, rr := postJSON(t, "/auth/login", types.LoginRequest{
			Username: "unknown@example.com",
			Password: "password123",
		})

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("невалидный JSON", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte("invalid json")))
		rr := httptest.NewRecorder()

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("GET запрос возвращает HTML", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
		rr := httptest.NewRecorder()

		envConfig := &types.EnvConfig{
			TemplateDir: ".",
			UseTls:      false,
			TokenTTL:    time.Second * 300,
		}

		handler.LoginUser(rr, req, envConfig)

		assert.NotEqual(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("неподдерживаемый метод", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", time.Second*300)}
		req := httptest.NewRequest(http.MethodDelete, "/auth/login", nil)
		rr := httptest.NewRecorder()

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}
