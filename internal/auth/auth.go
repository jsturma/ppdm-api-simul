package auth

import (
	"fmt"
	"strings"
	"sync"

	"ppdm-simul/internal/mock"
)

type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Scope        string
	TokenType    string
}

func (t TokenInfo) ToMap() map[string]any {
	return map[string]any{
		"access_token":  t.AccessToken,
		"refresh_token": t.RefreshToken,
		"expires_in":    t.ExpiresIn,
		"scope":         t.Scope,
		"token_type":    t.TokenType,
		"jti":           mock.NewUUID(),
	}
}

type Manager struct {
	enabled  bool
	tokenTTL int
	mu       sync.RWMutex
	tokens   map[string]TokenInfo
}

func NewManager(enabled bool, tokenTTL int) *Manager {
	return &Manager{
		enabled:  enabled,
		tokenTTL: tokenTTL,
		tokens:   make(map[string]TokenInfo),
	}
}

func (m *Manager) Login(username, password string) (TokenInfo, error) {
	if username == "" || password == "" {
		return TokenInfo{}, fmt.Errorf("username and password are required")
	}
	token := m.issueToken()
	m.mu.Lock()
	m.tokens[token.AccessToken] = token
	m.mu.Unlock()
	return token, nil
}

func (m *Manager) Refresh(refreshToken string) (TokenInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for access, existing := range m.tokens {
		if existing.RefreshToken == refreshToken {
			delete(m.tokens, access)
			token := m.issueToken()
			m.tokens[token.AccessToken] = token
			return token, nil
		}
	}
	return TokenInfo{}, fmt.Errorf("invalid refresh token")
}

func (m *Manager) Enabled() bool {
	return m.enabled
}

func (m *Manager) Validate(authorization string) bool {
	if !m.enabled {
		return true
	}
	token := bearerToken(authorization)
	if token == "" {
		return false
	}
	m.mu.RLock()
	_, ok := m.tokens[token]
	m.mu.RUnlock()
	return ok
}

func (m *Manager) Logout(authorization string) {
	token := bearerToken(authorization)
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.tokens, token)
	m.mu.Unlock()
}

func (m *Manager) issueToken() TokenInfo {
	suffix := mock.NewTokenSuffix()
	return TokenInfo{
		AccessToken:  fmt.Sprintf("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJwcGRtLXNpbXVsYXRvciJ9.%s", suffix),
		RefreshToken: fmt.Sprintf("eyJyZWZyZXNoIjp0cnVlfQ.%s.signature", suffix),
		ExpiresIn:    m.tokenTTL,
		Scope:        "admin",
		TokenType:    "Bearer",
	}
}

func bearerToken(authorization string) string {
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
