// Package secret шифрует ключи платёжных магазинов студий перед записью в базу.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var ErrNoKey = errors.New("encryption key is not configured")

// Box шифрует AES-256-GCM. Ключ — 32 байта в hex (PAYMENT_SECRET_KEY).
type Box struct {
	aead cipher.AEAD
}

// New принимает ключ в hex; пустой ключ — шифрование выключено (Ready() == false).
func New(hexKey string) (*Box, error) {
	hexKey = strings.TrimSpace(hexKey)
	if hexKey == "" {
		return &Box{}, nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("PAYMENT_SECRET_KEY must be 64 hex characters (openssl rand -hex 32)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Ready() bool { return b != nil && b.aead != nil }

func (b *Box) Seal(plain string) ([]byte, error) {
	if !b.Ready() {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (b *Box) Open(sealed []byte) (string, error) {
	if !b.Ready() {
		return "", ErrNoKey
	}
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("sealed value is too short")
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return "", errors.New("cannot decrypt: key changed or data damaged")
	}
	return string(plain), nil
}
