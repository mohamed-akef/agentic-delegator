//go:build saas

// cmd/agentic-delegator-saas/main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/uptrace/bun"
	bunmigrate "github.com/uptrace/bun/migrate"

	adcrypto "agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/adapter/clock"
	"agentic-delegator/core/adapter/docker"
	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/adapter/postgres"
	"agentic-delegator/core/adapter/webhook"
	"agentic-delegator/core/config"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
	pgmig "agentic-delegator/core/adapter/postgres/migrations"
	"agentic-delegator/saas"
	"agentic-delegator/saas/ghapp"
	"agentic-delegator/saas/signup"
	"agentic-delegator/saas/store"
	saasmig "agentic-delegator/saas/store/migrations"
	"agentic-delegator/saas/tenancy"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: agentic-delegator-saas <serve|migrate>")
	}
	cmdName := os.Args[1]

	cfg, err := config.Load()
	must("config", err)

	db, err := postgres.Open(cfg.DSN)
	must("db", err)
	defer db.Close()

	switch cmdName {
	case "migrate":
		runMigrateSaas(db)
		return
	case "serve":
		// fall through
	default:
		log.Fatalf("unknown cmd: %s", cmdName)
	}

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

	// SaaS-specific wiring
	identitiesRepo := store.NewIdentitiesRepo(db)
	installationsRepo := store.NewInstallationsRepo(db)
	sessionsRepo := store.NewSessionsRepo(db)
	sessions := signup.NewSessions(sessionsRepo)

	appCreds := ghapp.AppCreds{
		AppID:         envInt64("AGENTIC_GH_APP_ID"),
		PrivateKeyPEM: []byte(os.Getenv("AGENTIC_GH_APP_PRIVATE_KEY")),
	}
	appClient := ghapp.NewAppClient(appCreds)

	oauth := signup.NewOAuth(
		signup.OAuthConfig{
			ClientID:     os.Getenv("AGENTIC_GH_CLIENT_ID"),
			ClientSecret: os.Getenv("AGENTIC_GH_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("AGENTIC_GH_OAUTH_REDIRECT_URL"),
		},
		sessions, identitiesRepo, usersBootstrap, idg, clk, nil,
	)
	installHandler := ghapp.NewInstallHandler(os.Getenv("AGENTIC_GH_APP_SLUG"), sessions, installationsRepo, appClient)
	webhookHandler := ghapp.NewWebhookHandler([]byte(os.Getenv("AGENTIC_GH_WEBHOOK_SECRET")), installationsRepo)

	repoCreds := ghapp.NewRepoCredsProvider(appClient, installationsRepo)
	anthCreds := selfhost.NewAnthropicCredsProvider(&secrets)

	resolver := tenancy.NewResolver(sessions, apiKeys)
	edition := saas.New(resolver, repoCreds, anthCreds, oauth, installHandler, webhookHandler)
	_ = edition.Bootstrap(ctx)

	// Use cases — identical to selfhost
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
	dashHandler := adhttp.NewDashboardHandler(listJobs, apiKeys, &secrets)

	router := adhttp.NewRouter(adhttp.Deps{
		Resolver:        editionResolver{e: edition},
		JobsHandler:     jobsHandler,
		SettingsHandler: settingsHandler,
		StatusPage:      statusPage,
		Dashboard:       dashHandler,
		Edition:         edition,
	})

	log.Printf("listening on http://%s (saas mode)", cfg.HTTPBind)
	if err := http.ListenAndServe(cfg.HTTPBind, router); err != nil {
		log.Fatal(err)
	}
}

// editionResolver adapts saas.Edition into adhttp.UserResolver.
type editionResolver struct{ e *saas.Edition }

func (er editionResolver) Resolve(r *http.Request) (domain.UserID, error) {
	return er.e.ResolveUser(r)
}

func runMigrateSaas(db *bun.DB) {
	ctx := context.Background()
	// Run core migrations first
	mCore := bunmigrate.NewMigrator(db, pgmig.Migrations)
	_ = mCore.Init(ctx)
	if _, err := mCore.Migrate(ctx); err != nil {
		log.Fatalf("core migrate: %v", err)
	}
	// Then saas migrations (separate table to avoid clashing with core's bun_migrations)
	mSaas := bunmigrate.NewMigrator(db, saasmig.Migrations, bunmigrate.WithTableName("bun_saas_migrations"))
	_ = mSaas.Init(ctx)
	if _, err := mSaas.Migrate(ctx); err != nil {
		log.Fatalf("saas migrate: %v", err)
	}
	log.Println("migrations applied")
}

func envInt64(name string) int64 {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("env %s: %v", name, err)
	}
	return n
}

func must(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

// encryptingSecrets is a decorator that wraps SecretsRepository with AES-GCM
// at the composition seam. The Postgres impl stores bytes; this wrapper
// encrypts on write and decrypts on read.
type encryptingSecrets struct {
	inner ports.SecretsRepository
	aes   *adcrypto.AESGCM
}

// Compile-time assertion: encryptingSecrets satisfies ports.SecretsRepository.
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
