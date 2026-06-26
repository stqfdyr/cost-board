package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/scrypt"
)

const (
	scryptN      = 32768
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 32
)

type Credentials struct {
	Username string `json:"username"`
	Salt     string `json:"salt"`
	Hash     string `json:"hash"`
}

type Auth struct {
	mu          sync.RWMutex
	credentials *Credentials
	authPath    string
	sessions    map[string]time.Time
	sessionTTL  time.Duration
}

func New(dataDir string) (*Auth, error) {
	a := &Auth{
		authPath:   filepath.Join(dataDir, ".auth"),
		sessions:   make(map[string]time.Time),
		sessionTTL: 30 * 24 * time.Hour,
	}
	if err := a.load(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Auth) load() error {
	data, err := os.ReadFile(a.authPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read credentials: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("parse credentials: %w", err)
	}
	a.mu.Lock()
	a.credentials = &creds
	a.mu.Unlock()
	return nil
}

func (a *Auth) HasCredentials() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.credentials != nil
}

func (a *Auth) SetCredentials(username, password string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	hash, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	creds := Credentials{
		Username: username,
		Salt:     hex.EncodeToString(salt),
		Hash:     hex.EncodeToString(hash),
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(a.authPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(a.authPath, data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	a.mu.Lock()
	a.credentials = &creds
	a.mu.Unlock()
	return nil
}

func (a *Auth) Login(username, password string) (string, bool) {
	a.mu.RLock()
	creds := a.credentials
	a.mu.RUnlock()

	if creds == nil {
		return "", false
	}

	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(creds.Username)) == 1

	salt, err := hex.DecodeString(creds.Salt)
	if err != nil {
		return "", false
	}

	expectedHash, err := hex.DecodeString(creds.Hash)
	if err != nil {
		return "", false
	}

	hash, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return "", false
	}

	passMatch := subtle.ConstantTimeCompare(hash, expectedHash) == 1

	if !userMatch || !passMatch {
		return "", false
	}

	return a.createSession(), true
}

func (a *Auth) createSession() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	token := hex.EncodeToString(b)

	a.mu.Lock()
	a.sessions[token] = time.Now().Add(a.sessionTTL)
	a.mu.Unlock()

	return token
}

func (a *Auth) Validate(token string) bool {
	a.mu.RLock()
	expiry, ok := a.sessions[token]
	a.mu.RUnlock()

	if !ok {
		return false
	}

	if time.Now().After(expiry) {
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return false
	}

	return true
}

func (a *Auth) Logout(token string) {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *Auth) CleanupExpiredSessions() {
	a.mu.Lock()
	now := time.Now()
	for token, expiry := range a.sessions {
		if now.After(expiry) {
			delete(a.sessions, token)
		}
	}
	a.mu.Unlock()
}
