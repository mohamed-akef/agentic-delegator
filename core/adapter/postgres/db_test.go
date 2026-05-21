// core/adapter/postgres/db_test.go
//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"agentic-delegator/core/adapter/postgres"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DELEGATOR_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable"
	}
	return dsn
}

func TestOpen_ping(t *testing.T) {
	db, err := postgres.Open(testDSN(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
