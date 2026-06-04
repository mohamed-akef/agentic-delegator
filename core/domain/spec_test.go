// core/domain/spec_test.go
package domain_test

import (
	"testing"

	"agentic-delegator/core/domain"
)

func TestSpecSource_Valid(t *testing.T) {
	tests := []struct {
		name string
		s    domain.SpecSource
		want bool
	}{
		{"inline ok", domain.SpecSource{Type: domain.SourceTypeInline, Value: "hello"}, true},
		{"path ok", domain.SpecSource{Type: domain.SourceTypePath, Value: "specs/x.md"}, true},
		{"url ok", domain.SpecSource{Type: domain.SourceTypeURL, Value: "https://x/y.md"}, true},
		{"empty value", domain.SpecSource{Type: domain.SourceTypePath, Value: ""}, false},
		{"unknown type", domain.SpecSource{Type: "weird", Value: "x"}, false},
		{"zero value", domain.SpecSource{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.Valid(); got != tc.want {
				t.Fatalf("Valid: want %v, got %v", tc.want, got)
			}
		})
	}
}
