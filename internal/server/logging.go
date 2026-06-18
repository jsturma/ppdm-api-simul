package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxLoggedBodyBytes = 4096

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	if !s.logAPI {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqBody := readAndRestoreBody(r)

		operation := s.bundle.Match(r.Method, r.URL.Path)
		operationID := ""
		if operation != nil {
			operationID = operation.OperationID()
		}

		rec := &responseRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(rec, r)

		query := ""
		if r.URL.RawQuery != "" {
			query = "?" + r.URL.RawQuery
		}

		log.Printf(
			"--> %s %s%s op=%s auth=%s\n%s",
			r.Method,
			r.URL.Path,
			query,
			operationID,
			redactAuth(r.Header.Get("Authorization")),
			formatLoggedBody(reqBody),
		)
		log.Printf(
			"<-- %s %s %d %s\n%s",
			r.Method,
			r.URL.Path,
			rec.status,
			time.Since(start).Round(time.Millisecond),
			formatLoggedBody(rec.body.Bytes()),
		)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.body.Write(data)
	return r.ResponseWriter.Write(data)
}

func readAndRestoreBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		r.Body = http.NoBody
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body
}

func redactAuth(authorization string) string {
	if authorization == "" {
		return "-"
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "***"
	}
	token := strings.TrimSpace(parts[1])
	if len(token) <= 12 {
		return "Bearer ***"
	}
	return "Bearer " + token[:8] + "..." + token[len(token)-4:]
}

func formatLoggedBody(body []byte) string {
	if len(body) == 0 {
		return "  -"
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "  -"
	}

	if json.Valid(trimmed) {
		var payload any
		if err := json.Unmarshal(trimmed, &payload); err == nil {
			redacted := redactJSONValue(payload)
			encoded, err := json.Marshal(redacted)
			if err == nil {
				return truncateForLog(indentJSON(encoded))
			}
		}
	}

	return truncateForLog(indentOrRaw(trimmed))
}

func indentJSON(body []byte) []byte {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		return body
	}
	return pretty.Bytes()
}

func indentOrRaw(body []byte) []byte {
	if json.Valid(body) {
		return indentJSON(body)
	}
	return body
}

func redactJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveKey(key) {
				result[key] = "***"
				continue
			}
			result[key] = redactJSONValue(item)
		}
		return result
	case []any:
		items := make([]any, len(v))
		for i, item := range v {
			items[i] = redactJSONValue(item)
		}
		return items
	default:
		return v
	}
}

func isSensitiveKey(key string) bool {
	switch strings.ToLower(key) {
	case "password", "access_token", "refresh_token", "authorization", "client_secret", "secret":
		return true
	default:
		return false
	}
}

func truncateForLog(body []byte) string {
	if len(body) <= maxLoggedBodyBytes {
		return string(body)
	}
	return string(body[:maxLoggedBodyBytes]) + "\n  ...(truncated)"
}
