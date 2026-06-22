package extensions

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	managementSecretFile = "management.secret"
	proxyAPITokenFile    = "proxy-api.token"
)

func ensureCredential(path string, prefix string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" {
			return value, nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read credential: %w", err)
	}
	value, err := randomCredential(prefix)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		return "", fmt.Errorf("write credential: %w", err)
	}
	return value, nil
}

func readCredential(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func credentialPath(authDir string, name string) string {
	return filepath.Join(authDir, name)
}

func randomCredential(prefix string) (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate credential: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
