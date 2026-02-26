package handlers

import (
	"WebSocketChat/internal/auth"
	"WebSocketChat/internal/types"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type WsHandler struct {
	Upgrader    *websocket.Upgrader
	AuthService *auth.Service
}

func (h *WsHandler) CreateWsConnection(w http.ResponseWriter, r *http.Request, ws types.WsServer) {
	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Error with websocket connection: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	claims, ok := r.Context().Value("user").(*types.Claims)
	if !ok || claims == nil {
		log.Error("User not found in context")
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, "token expired"), time.Now().Add(time.Second))
		_ = conn.Close()
		return
	}
	log.Debugf("WebSocket connection for user: %s", claims.Username)
	log.Debugf("Client with address %s connected", conn.RemoteAddr().String())

	client := ws.NewClient(conn)
	connChan := ws.GetConnChan()
	connChan <- client
	go ws.ReadFromClient(client)
	go ws.WriteToClient(client)
}

func (h *WsHandler) RegisterUser(w http.ResponseWriter, r *http.Request, e *types.EnvConfig) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{"Method Not Allowed"})
		return
	}

	if r.Method == http.MethodGet {
		http.ServeFile(w, r, e.TemplateDir+"/register.html")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var req types.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{err.Error()})
		return
	}

	err := h.AuthService.Register(req.Email, req.Password, req.Username)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(types.RegisterResponse{"Register successfully"})
}

func (h *WsHandler) LoginUser(w http.ResponseWriter, r *http.Request, e *types.EnvConfig) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{"Method Not Allowed"})
		return
	}

	if r.Method == http.MethodGet {
		http.ServeFile(w, r, e.TemplateDir+"/login.html")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var req types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{err.Error()})
		return
	}

	err := h.AuthService.Login(req.Username, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{err.Error()})
		return
	}
	jwtToken, err := h.AuthService.GenerateToken(req.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(types.ErrorResponse{err.Error()})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    jwtToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   e.UseTls,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(e.TokenTTL.Seconds()),
	})

	loginResponse := types.LoginResponse{
		Username: req.Username,
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(loginResponse)
}

func (h *WsHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value("user").(*types.Claims)
	if !ok || claims == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(types.ErrorResponse{"Unauthorized"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(types.LoginResponse{Username: claims.Username})
}
