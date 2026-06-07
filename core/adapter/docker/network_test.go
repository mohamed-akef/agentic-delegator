// core/adapter/docker/network_test.go
package docker

import (
	"errors"
	"testing"
)

func TestCheckNetwork_emptyNameSkips(t *testing.T) {
	called := false
	err := checkNetwork("", func(string) error { called = true; return errors.New("should not run") })
	if err != nil {
		t.Fatalf("empty name must be a no-op, got: %v", err)
	}
	if called {
		t.Fatal("inspect must not be called when name is empty (filtering disabled)")
	}
}

func TestCheckNetwork_presentOK(t *testing.T) {
	err := checkNetwork("runner-net", func(n string) error {
		if n != "runner-net" {
			t.Fatalf("inspect got %q", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("present network must pass, got: %v", err)
	}
}

func TestCheckNetwork_setButAbsentFailsFast(t *testing.T) {
	err := checkNetwork("runner-net", func(string) error { return errors.New("no such network") })
	if err == nil {
		t.Fatal("a configured-but-absent network must fail fast (not silently run unfiltered)")
	}
}
