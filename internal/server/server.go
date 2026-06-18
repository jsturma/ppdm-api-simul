package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ppdm-simul/internal/auth"
	"ppdm-simul/internal/loader"
	"ppdm-simul/internal/mock"
)

type Server struct {
	bundle *loader.Bundle
	auth   *auth.Manager
	logAPI bool
}

func New(bundle *loader.Bundle, authManager *auth.Manager, logAPI bool) *Server {
	return &Server{
		bundle: bundle,
		auth:   authManager,
		logAPI: logAPI,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleRequest)
	return requireTLS(corsMiddleware(s.loggingMiddleware(mux)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/health" {
		s.handleRequest(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"operations":   len(s.bundle.Operations),
		"auth_enabled": s.auth.Enabled(),
	})
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := r.URL.Path
	operation := s.bundle.Match(r.Method, path)
	if operation == nil {
		s.writeError(w, nil, http.StatusNotFound, path, "The requested resource was not found.", "Not Found")
		return
	}

	if requiresAuth(operation) && !s.auth.Validate(r.Header.Get("Authorization")) {
		s.writeError(w, operation, http.StatusUnauthorized, path, "The authorization token is either invalid or expired.", "Unauthorized")
		return
	}

	switch {
	case isLogin(operation):
		s.handleLogin(w, r, operation, path)
		return
	case isTokenRefresh(operation):
		s.handleTokenRefresh(w, r, operation, path)
		return
	case isLogout(operation):
		s.auth.Logout(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	status, schema := operation.SuccessResponse()
	if status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response, ok := operation.ResponseContent(strconv.Itoa(status))
	if !ok {
		writeJSON(w, status, map[string]any{})
		return
	}

	if response.ContentType != "" && response.ContentType != "application/json" {
		writeBytes(w, status, response.ContentType, []byte{})
		return
	}

	generator := mock.New(operation.Schemas)
	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "pageSize", 100)

	var body any
	if schema != nil && looksPaginated(schema, operation.Schemas) {
		body = generator.Paginated(schema, response.Example, page, pageSize)
	} else {
		body = generator.FromResponse(schema, response.Example, 0)
	}
	if body == nil && (status == http.StatusOK || status == http.StatusCreated || status == http.StatusAccepted) {
		body = map[string]any{}
	}

	writeJSON(w, status, body)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request, operation *loader.Operation, path string) {
	payload, ok := decodeJSON(r)
	if !ok {
		s.writeError(w, operation, http.StatusBadRequest, path, "The request body is either invalid or is missing the required fields.", "Bad Request")
		return
	}

	username := fmt.Sprint(payload["username"])
	password := fmt.Sprint(payload["password"])
	token, err := s.auth.Login(username, password)
	if err != nil {
		s.writeError(w, operation, http.StatusBadRequest, path, "The request body is either invalid or is missing the required fields.", "Bad Request")
		return
	}
	writeJSON(w, http.StatusOK, s.tokenBody(operation, token))
}

func (s *Server) handleTokenRefresh(w http.ResponseWriter, r *http.Request, operation *loader.Operation, path string) {
	payload, ok := decodeJSON(r)
	if !ok {
		s.writeError(w, operation, http.StatusBadRequest, path, "The request body is either invalid or is missing the required fields.", "Bad Request")
		return
	}

	refreshToken := fmt.Sprint(payload["refresh_token"])
	token, err := s.auth.Refresh(refreshToken)
	if err != nil {
		s.writeError(w, operation, http.StatusUnauthorized, path, "The authorization token is either invalid or expired.", "Unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, s.tokenBody(operation, token))
}

func (s *Server) tokenBody(operation *loader.Operation, token auth.TokenInfo) map[string]any {
	response, ok := operation.ResponseContent("200")
	generator := mock.New(operation.Schemas)

	var body map[string]any
	if ok {
		if generated, ok := generator.FromResponse(response.Schema, response.Example, 0).(map[string]any); ok && generated != nil {
			body = generated
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	for key, value := range token.ToMap() {
		body[key] = value
	}
	return body
}

func (s *Server) writeError(w http.ResponseWriter, operation *loader.Operation, status int, path, message, reason string) {
	body := s.errorBody(operation, status, path, message, reason)
	writeJSON(w, status, body)
}

func (s *Server) errorBody(operation *loader.Operation, status int, path, message, reason string) map[string]any {
	body := map[string]any{
		"code":      status,
		"message":   message,
		"path":      path,
		"reason":    reason,
		"timestamp": time.Now().UnixMilli(),
	}

	if operation == nil {
		return body
	}

	response, ok := operation.ResponseContent(strconv.Itoa(status))
	if !ok || response.Schema == nil {
		return body
	}

	generator := mock.New(operation.Schemas)
	if generated, ok := generator.FromSchema(response.Schema, 0).(map[string]any); ok && generated != nil {
		for key, value := range generated {
			if _, exists := body[key]; !exists {
				body[key] = value
			}
		}
	}
	body["code"] = status
	body["message"] = message
	body["path"] = path
	body["reason"] = reason
	body["timestamp"] = time.Now().UnixMilli()
	return body
}

func requiresAuth(operation *loader.Operation) bool {
	if isLogin(operation) {
		return false
	}
	security := operation.Security()
	if security != nil && len(security) == 0 {
		return false
	}
	return true
}

func isLogin(operation *loader.Operation) bool {
	return operation.Method == http.MethodPost && strings.HasSuffix(operation.PathTemplate, "/login")
}

func isTokenRefresh(operation *loader.Operation) bool {
	return operation.Method == http.MethodPost && strings.HasSuffix(operation.PathTemplate, "/token")
}

func isLogout(operation *loader.Operation) bool {
	return operation.Method == http.MethodPost && strings.HasSuffix(operation.PathTemplate, "/logout")
}

func looksPaginated(schema map[string]any, schemas map[string]any) bool {
	if ref, ok := schema["$ref"].(string); ok {
		const prefix = "#/components/schemas/"
		if strings.HasPrefix(ref, prefix) {
			name := ref[len(prefix):]
			if target, ok := schemas[name].(map[string]any); ok {
				schema = target
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	_, ok := properties["content"]
	return ok
}

func decodeJSON(r *http.Request) (map[string]any, bool) {
	defer r.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, false
	}
	return payload, true
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeBytes(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"code":      403,
				"message":   "HTTPS is required.",
				"path":      r.URL.Path,
				"reason":    "Forbidden",
				"timestamp": time.Now().UnixMilli(),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
