// core/domain/errors_test.go
package domain_test

import (
	"errors"
	"testing"

	"agentic-delegator/core/domain"
)

func TestDomainErrors_distinct(t *testing.T) {
	all := []error{
		domain.ErrNotFound,
		domain.ErrConflict,
		domain.ErrForbidden,
		domain.ErrInvalidState,
		domain.ErrInvalidInput,
	}
	for i := range all {
		for j := range all {
			if i == j {
				continue
			}
			if errors.Is(all[i], all[j]) {
				t.Fatalf("errors at %d and %d compare equal but must be distinct", i, j)
			}
		}
	}
}

func TestDomainErrors_isAndAs(t *testing.T) {
	wrapped := errors.Join(domain.ErrNotFound, errors.New("user u_1"))
	if !errors.Is(wrapped, domain.ErrNotFound) {
		t.Fatalf("wrapped ErrNotFound should be detectable via errors.Is")
	}
}
