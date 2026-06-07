// cmd/agentic-delegator/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"

	"agentic-delegator/core/adapter/clock"
	"agentic-delegator/core/adapter/credentials"
	adcrypto "agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/adapter/docker"
	"agentic-delegator/core/adapter/ghapp"
	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/adapter/http/auth"
	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/adapter/keyhash"
	"agentic-delegator/core/adapter/postgres"
	pgmig "agentic-delegator/core/adapter/postgres/migrations"
	"agentic-delegator/core/adapter/webhook"
	"agentic-delegator/core/config"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: agentic-delegator <serve|migrate>")
	}
	cmdName := os.Args[1]

	cfg, err := config.Load()
	must("config", err)

	db, err := postgres.OpenWithPool(cfg.DSN, postgres.PoolConfig{
		MaxOpenConns: cfg.DBMaxOpenConns,
		MaxIdleConns: cfg.DBMaxIdleConns,
	})
	must("db", err)
	defer db.Close()

	switch cmdName {
	case "migrate":
		runMigrate(db, os.Args[2:])
	case "serve":
		runServe(cfg, db)
	default:
		log.Fatalf("unknown cmd: %s", cmdName)
	}
}

func runServe(cfg *config.Config, db *bun.DB) {
	must("config", cfg.ValidateForServe())

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx := context.Background()
	clk := clock.System{}
	idg := idgen.NanoID{}
	aes, err := adcrypto.NewAESGCM(cfg.MasterKey)
	must("aes", err)

	// Private log directory (0700) for per-job log files.
	must("log dir", os.MkdirAll(cfg.LogDir, 0o700))

	jobsRepo := postgres.NewJobsRepo(db)
	rawSecrets := postgres.NewSecretsRepo(db)
	secrets := encryptingSecrets{inner: rawSecrets, aes: aes}
	// bcrypt minted keys at the composition seam (mirrors encryptingSecrets):
	// MintAPIKey hands us the plaintext in Hash; the resolver bcrypt-compares
	// on read, so the write side must hash.
	apiKeys := keyhash.New(postgres.NewAPIKeysRepo(db))
	usersBootstrap := postgres.NewUsersBootstrapRepo(db)

	// Fail fast if egress filtering was opted into (AGENTIC_RUNNER_NETWORK set)
	// but the network is absent — better than silently running unfiltered.
	must("runner network preflight", docker.PreflightNetwork(ctx, cfg.RunnerNetwork))

	runner := docker.New(docker.Config{
		Image:          cfg.RunnerImage,
		CPUs:           "2",
		MemoryMB:       2048,
		Network:        cfg.RunnerNetwork,
		DNS:            cfg.RunnerDNS,
		WorkDirHost:    cfg.WorkDirHost,
		MaxJobDuration: cfg.MaxJobDuration,
	})
	hooks := webhook.New(&http.Client{Timeout: 10 * time.Second})

	identitiesRepo := postgres.NewIdentitiesRepo(db)
	installationsRepo := postgres.NewInstallationsRepo(db)
	sessionsRepo := postgres.NewSessionsRepo(db)
	sessions := auth.NewSessions(sessionsRepo, cfg.CookieSecure)

	appClient := ghapp.NewAppClient(ghapp.AppCreds{
		AppID:         cfg.GHAppID,
		PrivateKeyPEM: cfg.GHAppPrivateKey,
	})

	oauth := auth.NewOAuth(
		auth.OAuthConfig{
			ClientID:     cfg.GHClientID,
			ClientSecret: cfg.GHClientSecret,
			RedirectURL:  cfg.GHOAuthRedirectURL,
			CookieSecure: cfg.CookieSecure,
		},
		sessions, identitiesRepo, usersBootstrap, idg, clk, nil,
	)
	installHandler := ghapp.NewInstallHandler(cfg.GHAppSlug, sessions, installationsRepo, appClient)
	webhookHandler := ghapp.NewWebhookHandler(cfg.GHWebhookSecret, installationsRepo)

	repoCreds := ghapp.NewRepoCredsProvider(appClient, installationsRepo)
	anthCreds := credentials.NewAnthropicCredsProvider(&secrets)
	resolver := auth.NewResolver(sessions, apiKeys)

	enqueue := &usecase.EnqueueJob{
		Jobs:                 jobsRepo,
		RepoCreds:            repoCreds,
		AnthropicCreds:       anthCreds,
		Runner:               runner,
		IDGen:                idg,
		Clock:                clk,
		LogDir:               cfg.LogDir,
		MaxConcurrentPerUser: cfg.MaxConcurrentPerUser,
		MaxConcurrentGlobal:  cfg.MaxConcurrentGlobal,
	}
	getJob := &usecase.GetJob{Jobs: jobsRepo}
	listJobs := &usecase.ListJobs{Jobs: jobsRepo}
	dispatchWebhook := &usecase.DispatchCompletionWebhook{Dispatcher: hooks}
	complete := &usecase.HandleRunnerCompletion{Jobs: jobsRepo, Clock: clk, Webhook: dispatchWebhook}
	reattach := &usecase.ReattachRunningJobs{Jobs: jobsRepo, Runner: runner, Clock: clk}
	mint := &usecase.MintAPIKey{Keys: apiKeys, IDGen: idg, Clock: clk}
	revoke := &usecase.RevokeAPIKey{Keys: apiKeys}
	setAnth := &usecase.SetAnthropicCredentials{Secrets: &secrets}
	cancelJob := &usecase.CancelJob{Jobs: jobsRepo, Runner: runner, Clock: clk}

	// Completion runs in the runner's supervisor goroutine; give it its own
	// background context so it survives a request's lifecycle.
	enqueue.OnComplete = func(res ports.RunnerResult) {
		if err := complete.Execute(context.Background(), res); err != nil {
			slog.Error("handle runner completion", "job_id", string(res.JobID), "err", err)
		}
	}
	if err := reattach.Execute(ctx); err != nil {
		slog.Error("reattach running jobs", "err", err)
	}
	// Best-effort: reap secrets dirs orphaned by jobs that finished while the
	// orchestrator was down (reattach skips supervise for still-alive jobs, so
	// their cleanup never ran). Keep only dirs for jobs still genuinely running.
	if running, err := jobsRepo.ListByStatus(ctx, domain.JobStatusRunning); err != nil {
		slog.Error("orphan secrets sweep: list running", "err", err)
	} else {
		keep := make(map[string]bool, len(running))
		for _, j := range running {
			keep[string(j.ID)] = true
		}
		if removed := docker.SweepOrphanSecrets(cfg.WorkDirHost, keep); len(removed) > 0 {
			slog.Info("swept orphan secrets dirs", "count", len(removed))
		}
	}

	jobsHandler := adhttp.NewJobsHandler(enqueue, getJob, listJobs, cancelJob)
	settingsHandler := adhttp.NewSettingsHandler(setAnth, mint, revoke)
	statusPage := adhttp.NewStatusPage(getJob)
	dashHandler := adhttp.NewDashboardHandler(listJobs, apiKeys, &secrets, resolver)

	router := adhttp.NewRouter(adhttp.Deps{
		Resolver:        resolver,
		JobsHandler:     jobsHandler,
		SettingsHandler: settingsHandler,
		StatusPage:      statusPage,
		Dashboard:       dashHandler,
		Routes:          routeMounter{oauth: oauth, install: installHandler, webhook: webhookHandler},
		HealthCheck:     func(c context.Context) error { return db.PingContext(c) },
	})

	srv := &http.Server{
		Addr:              cfg.HTTPBind,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM: stop accepting connections and drain
	// in-flight requests before exiting.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		slog.Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown", "err", err)
		}
	}()

	slog.Info("listening", "addr", cfg.HTTPBind)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	slog.Info("server stopped")
}

// routeMounter mounts auth + GitHub-App routes. Living in the composition root
// keeps the http adapter from importing the auth/ghapp adapters.
type routeMounter struct {
	oauth   *auth.OAuth
	install *ghapp.InstallHandler
	webhook *ghapp.WebhookHandler
}

func (m routeMounter) RegisterRoutes(r chi.Router) {
	r.Get("/login", m.oauth.Login)
	r.Get("/auth/github/callback", m.oauth.Callback)
	r.Get("/auth/github-app/install", m.install.Install)
	r.Get("/auth/github-app/callback", m.install.Callback)
	r.Post("/webhooks/github", m.webhook.Handle)
}

func runMigrate(db *bun.DB, args []string) {
	cmd := "up"
	if len(args) > 0 {
		cmd = args[0]
	}
	ctx := context.Background()
	m := migrate.NewMigrator(db, pgmig.Migrations)
	switch cmd {
	case "init":
		must("init", m.Init(ctx))
		fmt.Println("migration tables initialized")
	case "up":
		must("init", m.Init(ctx))
		group, err := m.Migrate(ctx)
		must("migrate", err)
		if group.IsZero() {
			fmt.Println("no new migrations")
		} else {
			fmt.Printf("applied: %s\n", group)
		}
	case "down":
		group, err := m.Rollback(ctx)
		must("rollback", err)
		fmt.Printf("rolled back: %s\n", group)
	case "status":
		ms, err := m.MigrationsWithStatus(ctx)
		must("status", err)
		for _, mm := range ms {
			fmt.Printf("%s  applied=%v\n", mm.Name, !mm.MigratedAt.IsZero())
		}
	default:
		log.Fatalf("unknown migrate cmd %q", cmd)
	}
}

func must(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

// encryptingSecrets wraps SecretsRepository with AES-GCM at the composition
// seam: the Postgres impl stores bytes; this wrapper encrypts on write and
// decrypts on read.
type encryptingSecrets struct {
	inner ports.SecretsRepository
	aes   *adcrypto.AESGCM
}

var _ ports.SecretsRepository = (*encryptingSecrets)(nil)

func (e *encryptingSecrets) SetAnthropicCreds(ctx context.Context, userID domain.UserID, c domain.AnthropicCreds) error {
	ct, err := e.aes.Encrypt([]byte(c.APIKey))
	if err != nil {
		return err
	}
	return e.inner.SetAnthropicCreds(ctx, userID, domain.AnthropicCreds{APIKey: string(ct)})
}

func (e *encryptingSecrets) GetAnthropicCreds(ctx context.Context, userID domain.UserID) (domain.AnthropicCreds, error) {
	c, err := e.inner.GetAnthropicCreds(ctx, userID)
	if err != nil {
		return c, err
	}
	pt, err := e.aes.Decrypt([]byte(c.APIKey))
	if err != nil {
		return c, err
	}
	return domain.AnthropicCreds{APIKey: string(pt)}, nil
}

func (e *encryptingSecrets) DeleteAnthropicCreds(ctx context.Context, userID domain.UserID) error {
	return e.inner.DeleteAnthropicCreds(ctx, userID)
}
