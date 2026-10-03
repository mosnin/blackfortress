// Package secrets generates and persists the machine-local credentials that
// Black Fortress needs: probod's encryption and signing material plus the
// local user's password and API key.
package secrets

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

type Secrets struct {
	EncryptionKey    string `json:"encryption_key"`
	CookieSecret     string `json:"cookie_secret"`
	PasswordPepper   string `json:"password_pepper"`
	TrustTokenSecret string `json:"trust_token_secret"`
	OAuth2SigningKey string `json:"oauth2_signing_key"`
	PgPassword       string `json:"pg_password"`

	UserEmail       string    `json:"user_email"`
	UserPassword    string    `json:"user_password"`
	OrganizationID  string    `json:"organization_id,omitempty"`
	APIKey          string    `json:"api_key,omitempty"`
	APIKeyExpiresAt time.Time `json:"api_key_expires_at,omitzero"`

	// MCPToken authenticates agents to bfd's own MCP endpoint. It never
	// grants direct access to probod.
	MCPToken string `json:"mcp_token"`
}

// LoadOrCreate reads the secrets file, generating any missing value.
func LoadOrCreate(path string) (*Secrets, error) {
	s := &Secrets{}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("cannot parse %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	changed := false
	fill := func(field *string, gen func() (string, error)) error {
		if *field != "" {
			return nil
		}

		v, err := gen()
		if err != nil {
			return err
		}

		*field = v
		changed = true

		return nil
	}

	steps := []struct {
		field *string
		gen   func() (string, error)
	}{
		{&s.EncryptionKey, func() (string, error) { return randomBase64(32) }},
		{&s.CookieSecret, func() (string, error) { return randomBase64(48) }},
		{&s.PasswordPepper, func() (string, error) { return randomBase64(48) }},
		{&s.TrustTokenSecret, func() (string, error) { return randomBase64(48) }},
		{&s.OAuth2SigningKey, rsaKeyPEM},
		{&s.PgPassword, func() (string, error) { return randomURLSafe(24) }},
		{&s.UserEmail, func() (string, error) { return "owner@blackfortress.local", nil }},
		{&s.UserPassword, func() (string, error) { return randomURLSafe(24) }},
		{&s.MCPToken, func() (string, error) { return randomURLSafe(32) }},
	}
	for _, step := range steps {
		if err := fill(step.field, step.gen); err != nil {
			return nil, err
		}
	}

	if changed {
		if err := s.Save(path); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Secrets) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("cannot write secrets: %w", err)
	}

	return os.Rename(tmp, path)
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("cannot generate random bytes: %w", err)
	}

	return b, nil
}

func randomBase64(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

func randomURLSafe(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func rsaKeyPEM() (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("cannot generate RSA key: %w", err)
	}

	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}

	return string(pem.EncodeToMemory(block)), nil
}

// Load reads an existing secrets file without generating anything.
func Load(path string) (*Secrets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	s := &Secrets{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	return s, nil
}
