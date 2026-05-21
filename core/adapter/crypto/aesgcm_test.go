// core/adapter/crypto/aesgcm_test.go
package crypto_test

import (
	"bytes"
	"testing"

	"agentic-delegator/core/adapter/crypto"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	// 32-byte test key (AES-256)
	return bytes.Repeat([]byte{0xAB}, 32)
}

func TestAESGCM_roundTrip(t *testing.T) {
	c, err := crypto.NewAESGCM(newKey(t))
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	plaintext := []byte("sk-ant-secret-12345")
	ct, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ct, plaintext) {
		t.Fatalf("ciphertext equals plaintext")
	}
	pt, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("roundtrip mismatch: %q vs %q", pt, plaintext)
	}
}

func TestAESGCM_differentNoncePerEncrypt(t *testing.T) {
	c, _ := crypto.NewAESGCM(newKey(t))
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if bytes.Equal(a, b) {
		t.Fatalf("repeated encrypt of same plaintext should differ (nonce randomness)")
	}
}

func TestAESGCM_tamperedCiphertextFails(t *testing.T) {
	c, _ := crypto.NewAESGCM(newKey(t))
	ct, _ := c.Encrypt([]byte("hello"))
	ct[len(ct)-1] ^= 0x01 // flip a bit in the auth tag
	if _, err := c.Decrypt(ct); err == nil {
		t.Fatalf("tampered ciphertext should fail decryption")
	}
}

func TestAESGCM_rejectsBadKeyLength(t *testing.T) {
	if _, err := crypto.NewAESGCM([]byte{1, 2, 3}); err == nil {
		t.Fatalf("3-byte key should be rejected; want error")
	}
}
