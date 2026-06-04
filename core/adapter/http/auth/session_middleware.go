// core/adapter/http/auth/session_middleware.go
package auth

import (
	"encoding/hex"
)

func hexEncode(b []byte) string          { return hex.EncodeToString(b) }
func hexDecode(s string) ([]byte, error) { return hex.DecodeString(s) }
