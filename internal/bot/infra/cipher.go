package infra

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
)

type Cipher struct{ aead cipher.AEAD }

func NewCipher(cfg *config.Config) (*Cipher, error) {
	block, err := aes.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return &Cipher{aead: aead}, nil
}
func (c *Cipher) encrypt(key string, data []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, domain.ErrUnavailable
	}
	return c.aead.Seal(nonce, nonce, data, []byte(key)), nil
}
func (c *Cipher) decrypt(key string, data []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(data) < n+c.aead.Overhead() {
		return nil, domain.ErrUnavailable
	}
	p, err := c.aead.Open(nil, data[:n], data[n:], []byte(key))
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return p, nil
}
