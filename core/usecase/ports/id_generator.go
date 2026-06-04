// core/usecase/ports/id_generator.go
package ports

// IDGenerator is a port for generating new opaque identifiers and key
// material. Tests use a deterministic fake.
type IDGenerator interface {
	NewJobID() string
	NewAPIKeyID() string
	NewUserID() string

	// NewAPIKeyPlaintext returns a freshly generated API key (plaintext) and
	// its prefix (typically the first 8 characters, used for fast lookup).
	NewAPIKeyPlaintext() (plain string, prefix string)
}
