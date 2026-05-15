package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/otis/internal/state"
)

type TokenStore struct {
	stateDir string
}

func NewTokenStore(stateDir string) *TokenStore {
	return &TokenStore{
		stateDir: stateDir,
	}
}

func (s *TokenStore) Authorize(r *http.Request) (string, bool) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return "", false
	}
	tokens, err := s.load()
	if err != nil {
		return "", false
	}
	label, ok := tokens[tokenHash(token)]
	return label, ok
}

func (s *TokenStore) load() (map[string]string, error) {
	entries, err := os.ReadDir(state.TokensDir(s.stateDir))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	loaded := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".token" {
			name = strings.TrimSuffix(name, ".token")
		}
		if len(name) != sha256.Size*2 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(state.TokensDir(s.stateDir), entry.Name()))
		if err != nil {
			return nil, err
		}
		loaded[name] = strings.TrimSpace(string(raw))
	}
	return loaded, nil
}

func IssueToken(stateDir string, label string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	hash := tokenHash(token)
	if err := os.MkdirAll(state.TokensDir(stateDir), 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(state.TokensDir(stateDir), hash+".token")
	return token, os.WriteFile(path, []byte(strings.TrimSpace(label)+"\n"), 0o600)
}

func bearerToken(header string) string {
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
