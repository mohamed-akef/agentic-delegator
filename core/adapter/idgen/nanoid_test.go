// core/adapter/idgen/nanoid_test.go
package idgen_test

import (
	"strings"
	"testing"

	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/usecase/ports"
)

func TestNanoID_satisfiesPort(t *testing.T) {
	var _ ports.IDGenerator = idgen.NanoID{}
}

func TestNanoID_NewJobID_prefixedAndUnique(t *testing.T) {
	g := idgen.NanoID{}
	a := g.NewJobID()
	b := g.NewJobID()
	if !strings.HasPrefix(a, "j_") {
		t.Fatalf("job id must start with j_, got %s", a)
	}
	if a == b {
		t.Fatalf("two job ids should differ, got %s == %s", a, b)
	}
	if len(a) < 8 {
		t.Fatalf("job id too short: %s", a)
	}
}

func TestNanoID_NewAPIKeyIDAndUserID(t *testing.T) {
	g := idgen.NanoID{}
	if !strings.HasPrefix(g.NewAPIKeyID(), "k_") {
		t.Fatal("api key id must start with k_")
	}
	if !strings.HasPrefix(g.NewUserID(), "u_") {
		t.Fatal("user id must start with u_")
	}
}

func TestNanoID_NewAPIKeyPlaintext(t *testing.T) {
	g := idgen.NanoID{}
	plain, prefix := g.NewAPIKeyPlaintext()
	if !strings.HasPrefix(plain, "agdkey_") {
		t.Fatalf("plaintext must start with agdkey_, got %s", plain)
	}
	if len(prefix) != 8 {
		t.Fatalf("prefix must be 8 chars, got %d (%s)", len(prefix), prefix)
	}
	if !strings.HasPrefix(plain, prefix) {
		t.Fatalf("prefix must match start of plaintext")
	}
	if len(plain) < 24 {
		t.Fatalf("plaintext too short to be secure: %s", plain)
	}
}
