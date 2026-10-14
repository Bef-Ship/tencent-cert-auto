package cert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/registration"
)

// User implements lego registration.User and persists ACME account state.
type User struct {
	Email        string                 `json:"email"`
	Registration *registration.Resource `json:"registration"`
	key          crypto.PrivateKey
	keyPath      string
	metaPath     string
}

func loadOrCreateUser(dir, email string) (*User, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	u := &User{
		Email:    email,
		keyPath:  filepath.Join(dir, "account.key"),
		metaPath: filepath.Join(dir, "account.json"),
	}

	if err := u.loadKey(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		u.key = key
		if err := u.saveKey(); err != nil {
			return nil, err
		}
	}

	_ = u.loadMeta()
	return u, nil
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.key }

func (u *User) saveMeta() error {
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.metaPath, data, 0o600)
}

func (u *User) loadMeta() error {
	data, err := os.ReadFile(u.metaPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, u)
}

func (u *User) saveKey() error {
	priv, ok := u.key.(*ecdsa.PrivateKey)
	if !ok {
		return os.ErrInvalid
	}
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	return os.WriteFile(u.keyPath, pem.EncodeToMemory(block), 0o600)
}

func (u *User) loadKey() error {
	data, err := os.ReadFile(u.keyPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return os.ErrInvalid
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	u.key = key
	return nil
}
