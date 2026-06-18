package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	baseURL := envOr("PPDM_URL", "https://localhost:8443")
	username := envOr("PPDM_USER", "admin")
	password := envOr("PPDM_PASSWORD", "admin")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	fmt.Printf("Logging in to %s...\n", baseURL)
	loginBody, token, err := login(client, baseURL, username, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Login response:")
	fmt.Println(prettyJSON(loginBody))

	fmt.Println("Fetching assets...")
	assetsBody, err := getAssets(client, baseURL, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get assets failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Assets response:")
	fmt.Println(prettyJSON(assetsBody))
}

func login(client *http.Client, baseURL, username, password string) ([]byte, string, error) {
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v2/login", strings.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status %s: %s", resp.Status, string(raw))
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", err
	}
	token, _ := payload["access_token"].(string)
	if token == "" {
		return nil, "", fmt.Errorf("access_token missing in login response")
	}
	return raw, token, nil
}

func getAssets(client *http.Client, baseURL, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v2/assets?page=1&pageSize=10", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, string(raw))
	}
	return raw, nil
}

func prettyJSON(raw []byte) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return string(raw)
	}
	return pretty.String()
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
