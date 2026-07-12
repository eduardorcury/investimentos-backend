package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/duducury/investimentos-backend/internal/auth"
)

type AuthHandler struct {
	service *auth.Service
}

func NewAuthHandler(service *auth.Service) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	slog.Info("login requested")

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Warn("failed to decode login body", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	token, err := h.service.Login(body.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		slog.Warn("login failed: invalid credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	case err != nil:
		slog.Error("login failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
	default:
		slog.Info("login succeeded")
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
	}
}
