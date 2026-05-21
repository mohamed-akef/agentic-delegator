// cmd/agentic-delegator/migrate/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/uptrace/bun/migrate"

	"agentic-delegator/core/adapter/postgres"
	pgmig "agentic-delegator/core/adapter/postgres/migrations"
)

func main() {
	cmd := flag.String("cmd", "up", "one of: up | down | status | init")
	dsn := flag.String("dsn", os.Getenv("DELEGATOR_DSN"), "postgres DSN (or DELEGATOR_DSN env)")
	flag.Parse()
	if *dsn == "" {
		// allow positional subcommand: `migrate up`
		args := flag.Args()
		if len(args) > 0 {
			*cmd = args[0]
		}
		fallback := "postgres://delegator:delegator@127.0.0.1:5433/delegator?sslmode=disable"
		fmt.Fprintln(os.Stderr, "DELEGATOR_DSN not set; falling back to "+fallback)
		*dsn = fallback
	}

	db, err := postgres.Open(*dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	m := migrate.NewMigrator(db, pgmig.Migrations)
	switch *cmd {
	case "init":
		if err := m.Init(ctx); err != nil {
			log.Fatalf("init: %v", err)
		}
		fmt.Println("migration tables initialized")
	case "up":
		if err := m.Init(ctx); err != nil {
			log.Fatalf("init: %v", err)
		}
		group, err := m.Migrate(ctx)
		if err != nil {
			log.Fatalf("migrate: %v", err)
		}
		if group.IsZero() {
			fmt.Println("no new migrations")
		} else {
			fmt.Printf("applied: %s\n", group)
		}
	case "down":
		group, err := m.Rollback(ctx)
		if err != nil {
			log.Fatalf("rollback: %v", err)
		}
		fmt.Printf("rolled back: %s\n", group)
	case "status":
		ms, err := m.MigrationsWithStatus(ctx)
		if err != nil {
			log.Fatalf("status: %v", err)
		}
		for _, mm := range ms {
			fmt.Printf("%s  applied=%v\n", mm.Name, !mm.MigratedAt.IsZero())
		}
	default:
		log.Fatalf("unknown cmd %q", *cmd)
	}
}
