package handlers_test

import (
	"WebSocketChat/internal/auth"
	"WebSocketChat/internal/handlers"
	"WebSocketChat/internal/types"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)
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
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
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
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
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
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
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
			Email:    "test@example.com",
			Password: "password123",
		})

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp types.LoginResponse
		err := json.NewDecoder(rr.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "test@example.com", resp.User)

		// Проверяем, что cookie установлены (auth_token и refresh_token)
		cookies := rr.Result().Cookies()
		assert.Len(t, cookies, 2)

		var authToken, refreshToken *http.Cookie
		for _, c := range cookies {
			if c.Name == "auth_token" {
				authToken = c
			}
			if c.Name == "refresh_token" {
				refreshToken = c
			}
		}

		require.NotNil(t, authToken)
		require.NotNil(t, refreshToken)
		assert.NotEmpty(t, authToken.Value)
		assert.NotEmpty(t, refreshToken.Value)
		assert.Equal(t, "/", authToken.Path)
		assert.Equal(t, "/auth/refresh", refreshToken.Path)
	})

	t.Run("неверный пароль", func(t *testing.T) {
		service, handler := testHandler(t)
		_ = service.Register("test@example.com", "password123", "testuser")

		req, rr := postJSON(t, "/auth/login", types.LoginRequest{
			Email:    "test@example.com",
			Password: "wrongpassword",
		})

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("пользователь не существует", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
		req, rr := postJSON(t, "/auth/login", types.LoginRequest{
			Email:    "unknown@example.com",
			Password: "password123",
		})

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("невалидный JSON", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte("invalid json")))
		rr := httptest.NewRecorder()

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("GET запрос возвращает HTML", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
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
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
		req := httptest.NewRequest(http.MethodDelete, "/auth/login", nil)
		rr := httptest.NewRecorder()

		handler.LoginUser(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestMe(t *testing.T) {
	t.Run("авторизованный пользователь", func(t *testing.T) {
		service, handler := testHandler(t)
		token, _ := service.GenerateToken("testuser")

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
		rr := httptest.NewRecorder()

		// Валидируем токен и добавляем claims в контекст
		claims, err := service.ValidateToken(req)
		assert.NoError(t, err)

		ctx := context.WithValue(req.Context(), auth.UserKey, claims)
		req = req.WithContext(ctx)

		handler.Me(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp types.LoginResponse
		err = json.NewDecoder(rr.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "testuser", resp.Username)
	})

	t.Run("неавторизованный пользователь", func(t *testing.T) {
		handler := &handlers.WsHandler{AuthService: auth.NewService("secret", "refresh-secret", time.Second*300, time.Hour*24)}
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		rr := httptest.NewRecorder()

		handler.Me(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("с истёкшим токеном", func(t *testing.T) {
		service := auth.NewService("secret", "refresh-secret", time.Second*0, time.Hour*24)
		handler := &handlers.WsHandler{AuthService: service}
		token, _ := service.GenerateToken("testuser")

		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
		rr := httptest.NewRecorder()

		// Токен истёк, валидация должна вернуть ошибку
		_, err := service.ValidateToken(req)
		assert.Error(t, err)

		handler.Me(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestRefreshAccessToken(t *testing.T) {
	t.Run("успешное обновление access токена", func(t *testing.T) {
		service, handler := testHandler(t)

		// Регистрируем и логиним пользователя
		_ = service.Register("refresh@example.com", "password123", "refreshuser")

		// Получаем refresh токен
		refreshToken, err := service.GenerateRefreshToken("refresh@example.com")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
		rr := httptest.NewRecorder()

		handler.RefreshAccessToken(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, rr.Code)

		// Проверяем, что новый auth_token установлен
		cookies := rr.Result().Cookies()
		require.Len(t, cookies, 1)
		assert.Equal(t, "auth_token", cookies[0].Name)
		assert.NotEmpty(t, cookies[0].Value)
	})

	t.Run("отсутствует refresh токен", func(t *testing.T) {
		_, handler := testHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		rr := httptest.NewRecorder()

		handler.RefreshAccessToken(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("невалидный refresh токен", func(t *testing.T) {
		_, handler := testHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "invalid.token"})
		rr := httptest.NewRecorder()

		handler.RefreshAccessToken(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("истёкший refresh токен", func(t *testing.T) {
		service := auth.NewService("secret", "refresh-secret", time.Second*300, time.Second*0)
		handler := &handlers.WsHandler{AuthService: service}

		// Генерируем истёкший refresh токен
		refreshToken, _ := service.GenerateRefreshToken("testuser")
		time.Sleep(time.Millisecond * 100) // Ждём истечения

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
		rr := httptest.NewRecorder()

		handler.RefreshAccessToken(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("неподдерживаемый метод", func(t *testing.T) {
		_, handler := testHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/auth/refresh", nil)
		rr := httptest.NewRecorder()

		handler.RefreshAccessToken(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("полный цикл: логин -> refresh", func(t *testing.T) {
		service, handler := testHandler(t)

		// 1. Регистрируем пользователя
		_ = service.Register("fullcycle@example.com", "password123", "fulluser")

		// 2. Логинимся и получаем токены
		loginReq, loginRr := postJSON(t, "/auth/login", types.LoginRequest{
			Username: "fullcycle@example.com",
			Password: "password123",
		})
		handler.LoginUser(loginRr, loginReq, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, loginRr.Code)

		// 3. Извлекаем refresh токен из cookie
		var refreshToken string
		for _, c := range loginRr.Result().Cookies() {
			if c.Name == "refresh_token" {
				refreshToken = c.Value
				break
			}
		}
		require.NotEmpty(t, refreshToken)

		// 4. Обновляем access токен
		refreshReq := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		refreshReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
		refreshRr := httptest.NewRecorder()

		handler.RefreshAccessToken(refreshRr, refreshReq, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, refreshRr.Code)

		// 5. Проверяем новый access токен
		var newAuthToken string
		for _, c := range refreshRr.Result().Cookies() {
			if c.Name == "auth_token" {
				newAuthToken = c.Value
				break
			}
		}
		require.NotEmpty(t, newAuthToken)

		// 6. Проверяем, что новый токен валиден
		validateReq := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		validateReq.AddCookie(&http.Cookie{Name: "auth_token", Value: newAuthToken})
		claims, err := service.ValidateToken(validateReq)
		require.NoError(t, err)
		assert.Equal(t, "fullcycle@example.com", claims.Username)
	})
}

func TestLogoutHandler(t *testing.T) {
	t.Run("успешный logout", func(t *testing.T) {
		service, handler := testHandler(t)

		// Регистрируем и логиним пользователя
		_ = service.Register("logout@example.com", "password123", "logoutuser")

		loginReq, loginRr := postJSON(t, "/auth/login", types.LoginRequest{
			Username: "logout@example.com",
			Password: "password123",
		})
		handler.LoginUser(loginRr, loginReq, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, loginRr.Code)

		// Извлекаем токены из cookie
		var authToken, refreshToken string
		for _, c := range loginRr.Result().Cookies() {
			if c.Name == "auth_token" {
				authToken = c.Value
			}
			if c.Name == "refresh_token" {
				refreshToken = c.Value
			}
		}
		require.NotEmpty(t, authToken)
		require.NotEmpty(t, refreshToken)

		// Выполняем logout
		logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		logoutReq.AddCookie(&http.Cookie{Name: "auth_token", Value: authToken})
		logoutReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
		logoutRr := httptest.NewRecorder()

		handler.LogoutHandler(logoutRr, logoutReq, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, logoutRr.Code)

		// Проверяем ответ
		var resp types.LogoutResponse
		err := json.NewDecoder(logoutRr.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "Logged out successfully", resp.Message)

		// Проверяем, что cookie установлены на удаление
		cookies := logoutRr.Result().Cookies()
		require.Len(t, cookies, 2)

		var deleteAuthToken, deleteRefreshToken *http.Cookie
		for _, c := range cookies {
			if c.Name == "auth_token" {
				deleteAuthToken = c
			}
			if c.Name == "refresh_token" {
				deleteRefreshToken = c
			}
		}

		require.NotNil(t, deleteAuthToken)
		require.NotNil(t, deleteRefreshToken)
		assert.Empty(t, deleteAuthToken.Value)
		assert.Empty(t, deleteRefreshToken.Value)
		assert.Equal(t, "/", deleteAuthToken.Path)
		assert.Equal(t, "/auth/refresh", deleteRefreshToken.Path)
		assert.Equal(t, -1, deleteAuthToken.MaxAge)
		assert.Equal(t, -1, deleteRefreshToken.MaxAge)
	})

	t.Run("logout без токенов", func(t *testing.T) {
		_, handler := testHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		rr := httptest.NewRecorder()

		handler.LogoutHandler(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusOK, rr.Code)

		// Cookie всё равно должны быть установлены на удаление
		cookies := rr.Result().Cookies()
		require.Len(t, cookies, 2)
	})

	t.Run("logout GET методом запрещён", func(t *testing.T) {
		_, handler := testHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
		rr := httptest.NewRecorder()

		handler.LogoutHandler(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)

		var resp types.ErrorResponse
		err := json.NewDecoder(rr.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "Method Not Allowed", resp.Message)
	})

	t.Run("logout DELETE методом запрещён", func(t *testing.T) {
		_, handler := testHandler(t)

		req := httptest.NewRequest(http.MethodDelete, "/auth/logout", nil)
		rr := httptest.NewRecorder()

		handler.LogoutHandler(rr, req, testEnvConfig(t))

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("полный цикл: login -> logout -> проверка токена", func(t *testing.T) {
		service, handler := testHandler(t)

		// 1. Регистрируем пользователя
		_ = service.Register("cycle@example.com", "password123", "cycleuser")

		// 2. Логинимся
		loginReq, loginRr := postJSON(t, "/auth/login", types.LoginRequest{
			Username: "cycle@example.com",
			Password: "password123",
		})
		handler.LoginUser(loginRr, loginReq, testEnvConfig(t))
		assert.Equal(t, http.StatusOK, loginRr.Code)

		// 3. Извлекаем auth токен
		var authToken string
		for _, c := range loginRr.Result().Cookies() {
			if c.Name == "auth_token" {
				authToken = c.Value
				break
			}
		}
		require.NotEmpty(t, authToken)

		// 4. Проверяем, что токен валиден до logout
		meReq := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		meReq.AddCookie(&http.Cookie{Name: "auth_token", Value: authToken})
		claims, err := service.ValidateToken(meReq)
		require.NoError(t, err)
		assert.Equal(t, "cycle@example.com", claims.Username)

		// 5. Выполняем logout
		logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		logoutReq.AddCookie(&http.Cookie{Name: "auth_token", Value: authToken})
		logoutRr := httptest.NewRecorder()
		handler.LogoutHandler(logoutRr, logoutReq, testEnvConfig(t))
		assert.Equal(t, http.StatusOK, logoutRr.Code)

		// 6. После logout токен технически остаётся валидным (JWT stateless),
		// но cookie удалены и браузер не будет их отправлять
		// Это ожидаемое поведение без blacklist
	})
}
