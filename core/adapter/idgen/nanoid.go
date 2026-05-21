// core/adapter/idgen/nanoid.go
package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// NanoID generates URL-safe random identifiers via crypto/rand.
// Format: <prefix>_<base32-of-9-random-bytes> → e.g. "j_AB12CD34EF56789".
type NanoID struct{}

const apiKeyBytes = 24 // 24 random bytes → 192 bits → base32 ≈ 39 chars

func randBase32(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is catastrophic; panic is acceptable here.
		panic("idgen: crypto/rand failed: " + err.Error())
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return strings.ToLower(enc)
}

func (NanoID) NewJobID() string    { return "j_" + randBase32(9) }
func (NanoID) NewAPIKeyID() string { return "k_" + randBase32(9) }
func (NanoID) NewUserID() string   { return "u_" + randBase32(9) }

func (NanoID) NewAPIKeyPlaintext() (string, string) {
	plain := "agdkey_" + randBase32(apiKeyBytes)
	return plain, plain[:8]
}
