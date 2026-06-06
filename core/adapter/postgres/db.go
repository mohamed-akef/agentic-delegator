// core/adapter/postgres/db.go
package postgres

import (
	"database/sql"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// PoolConfig tunes the underlying sql.DB connection pool.
type PoolConfig struct {
	MaxOpenConns int
	MaxIdleConns int
}

// Open opens a Bun DB against a Postgres DSN with default pool settings.
// Example DSN: postgres://user:pass@host:5432/db?sslmode=disable
func Open(dsn string) (*bun.DB, error) {
	return OpenWithPool(dsn, PoolConfig{MaxOpenConns: 20, MaxIdleConns: 5})
}

// OpenWithPool opens a Bun DB and applies the given pool limits. Bounding open
// connections protects Postgres from connection exhaustion under load.
func OpenWithPool(dsn string, pool PoolConfig) (*bun.DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	if pool.MaxOpenConns > 0 {
		sqldb.SetMaxOpenConns(pool.MaxOpenConns)
	}
	if pool.MaxIdleConns > 0 {
		sqldb.SetMaxIdleConns(pool.MaxIdleConns)
	}
	sqldb.SetConnMaxLifetime(time.Hour)
	sqldb.SetConnMaxIdleTime(15 * time.Minute)
	return bun.NewDB(sqldb, pgdialect.New()), nil
}
