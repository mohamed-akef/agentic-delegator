// cmd/agentic-delegator/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
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

	db, err := postgres.Open(cfg.DSN)
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
	ctx := context.Background()
	clk := clock.System{}
	idg := idgen.NanoID{}
	aes, err := adcrypto.NewAESGCM(cfg.MasterKey)
	must("aes", err)

	jobsRepo := postgres.NewJobsRepo(db)
	rawSecrets := postgres.NewSecretsRepo(db)
	secrets := encryptingSecrets{inner: rawSecrets, aes: aes}
	apiKeys := postgres.NewAPIKeysRepo(db)
	usersBootstrap := postgres.NewUsersBootstrapRepo(db)

	runner := docker.New(docker.Config{Image: cfg.RunnerImage, CPUs: "2", MemoryMB: 2048})
	hooks := webhook.New(&http.Client{Timeout: 10 * time.Second})

	identitiesRepo := postgres.NewIdentitiesRepo(db)
	installationsRepo := postgres.NewInstallationsRepo(db)
	sessionsRepo := postgres.NewSessionsRepo(db)
	sessions := auth.NewSessions(sessionsRepo)

	appClient := ghapp.NewAppClient(ghapp.AppCreds{
		AppID:         cfg.GHAppID,
		PrivateKeyPEM: cfg.GHAppPrivateKey,
	})

	oauth := auth.NewOAuth(
		auth.OAuthConfig{
			ClientID:     cfg.GHClientID,
			ClientSecret: cfg.GHClientSecret,
			RedirectURL:  cfg.GHOAuthRedirectURL,
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
		MaxConcurrentPerUser: cfg.MaxConcurrentPerUser,
		MaxConcurrentGlobal:  cfg.MaxConcurrentGlobal,
	}
	getJob := &usecase.GetJob{Jobs: jobsRepo}
	listJobs := &usecase.ListJobs{Jobs: jobsRepo}
	complete := &usecase.HandleRunnerCompletion{Jobs: jobsRepo, Clock: clk}
	reattach := &usecase.ReattachRunningJobs{Jobs: jobsRepo, Runner: runner, Clock: clk}
	mint := &usecase.MintAPIKey{Keys: apiKeys, IDGen: idg, Clock: clk}
	revoke := &usecase.RevokeAPIKey{Keys: apiKeys}
	setAnth := &usecase.SetAnthropicCredentials{Secrets: &secrets}
	_ = &usecase.DispatchCompletionWebhook{Dispatcher: hooks}

	enqueue.OnComplete = func(res ports.RunnerResult) { _ = complete.Execute(ctx, res) }
	_ = reattach.Execute(ctx)

	jobsHandler := adhttp.NewJobsHandler(enqueue, getJob, listJobs)
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
	})

	log.Printf("listening on http://%s", cfg.HTTPBind)
	if err := http.ListenAndServe(cfg.HTTPBind, router); err != nil {
		log.Fatal(err)
	}
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
