// core/adapter/crypto/aesgcm.go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// AESGCM implements authenticated encryption (AES-256-GCM) for secrets at rest.
// Layout of ciphertext: [12-byte nonce][AES-GCM sealed bytes].
type AESGCM struct {
	gcm cipher.AEAD
}

func NewAESGCM(key []byte) (*AESGCM, error) {
	if l := len(key); l != 16 && l != 24 && l != 32 {
		return nil, fmt.Errorf("aesgcm: invalid key length %d (want 16, 24, or 32)", l)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{gcm: gcm}, nil
}

func (c *AESGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := c.gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

func (c *AESGCM) Decrypt(ciphertext []byte) ([]byte, error) {
	ns := c.gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("aesgcm: ciphertext too short")
	}
	nonce := ciphertext[:ns]
	sealed := ciphertext[ns:]
	return c.gcm.Open(nil, nonce, sealed, nil)
}
