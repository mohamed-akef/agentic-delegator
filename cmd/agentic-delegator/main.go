// cmd/agentic-delegator/main.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"agentic-delegator/core/adapter/clock"
	"agentic-delegator/core/adapter/crypto"
	"agentic-delegator/core/adapter/docker"
	adhttp "agentic-delegator/core/adapter/http"
	"agentic-delegator/core/adapter/idgen"
	"agentic-delegator/core/adapter/postgres"
	selfhost_pg "agentic-delegator/core/adapter/postgres/selfhost"
	"agentic-delegator/core/adapter/webhook"
	"agentic-delegator/core/config"
	"agentic-delegator/core/domain"
	"agentic-delegator/core/runtime/selfhost"
	"agentic-delegator/core/usecase"
	"agentic-delegator/core/usecase/ports"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			runInit()
			return
		case "serve":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			runServe()
			return
		case "reset-key":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			runResetKey()
			return
		}
	}
	fmt.Fprintln(os.Stderr, "usage: agentic-delegator <init|serve|reset-key>")
	os.Exit(2)
}

// ----- init -----

func runInit() {
	flag.Parse()
	cfg, err := config.Load()
	must("config.Load", err)
	db, err := postgres.Open(cfg.DSN)
	must("postgres.Open", err)
	defer db.Close()

	ctx := context.Background()

	// Ensure migrate has been run (best-effort: trying to insert the admin user
	// surfaces "table does not exist" if not).
	if _, err := db.ExecContext(ctx, `SELECT 1 FROM users LIMIT 1`); err != nil {
		log.Fatalf("schema not initialized — run `agentic-delegator migrate` first: %v", err)
	}

	// Generate the admin API key (plaintext shown once)
	plain := newAdminKeyPlaintext()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	must("bcrypt", err)

	// Persist hash + ensure admin row + clear PAT
	users := postgres.NewUsersBootstrapRepo(db)
	if err := users.UpsertAdmin(ctx, selfhost.AdminUserID, "admin", time.Now().UTC()); err != nil {
		log.Fatalf("UpsertAdmin: %v", err)
	}
	keys := postgres.NewAPIKeysRepo(db)
	// Wipe any existing admin keys for re-init
	if existing, err := keys.ListForUser(ctx, selfhost.AdminUserID); err == nil {
		for _, k := range existing {
			_ = keys.Delete(ctx, k.ID, selfhost.AdminUserID)
		}
	}
	id := idgen.NanoID{}.NewAPIKeyID()
	prefix := plain[:8]
	if err := keys.Create(ctx, domain.NewAPIKey(domain.APIKeyID(id), selfhost.AdminUserID, "admin", prefix, domain.APIKeyHash(hash), time.Now().UTC())); err != nil {
		log.Fatalf("Create api key: %v", err)
	}

	fmt.Println("admin API key (saved once — copy now):")
	fmt.Println(plain)
}

func newAdminKeyPlaintext() string {
	// 24 random bytes → 48 hex chars; with the 13-char prefix the total is 61
	// bytes, which fits under bcrypt's 72-byte ceiling.
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return "agdkey_admin_" + hex.EncodeToString(b)
}

// ----- serve -----

func runServe() {
	cfg, err := config.Load()
	must("config.Load", err)

	db, err := postgres.Open(cfg.DSN)
	must("postgres.Open", err)
	defer db.Close()

	ctx := context.Background()

	// outbound adapters
	clk := clock.System{}
	idg := idgen.NanoID{}
	aes, err := crypto.NewAESGCM(cfg.MasterKey)
	must("crypto.NewAESGCM", err)

	jobsRepo := postgres.NewJobsRepo(db)
	rawSecrets := postgres.NewSecretsRepo(db)
	secrets := newEncryptingSecrets(rawSecrets, aes) // see helper at bottom of this file
	apiKeys := postgres.NewAPIKeysRepo(db)
	usersBootstrap := postgres.NewUsersBootstrapRepo(db)
	patStore := selfhost_pg.NewSelfhostPATStore(db, aes)
	runner := docker.New(docker.Config{
		Image:    cfg.RunnerImage,
		CPUs:     "2",
		MemoryMB: 2048,
	})
	hooks := webhook.New(&http.Client{Timeout: 10 * time.Second})

	// Load existing admin key hash (from the only api_keys row for AdminUserID)
	adminHash := loadAdminKeyHash(ctx, apiKeys)

	// Edition
	repoCreds := selfhost.NewRepoCredsProvider(patStore)
	anthCreds := selfhost.NewAnthropicCredsProvider(secrets)
	bootstrap := selfhost.NewAdminBootstrap(usersBootstrap, clk, patStore, adminHash)
	edition := selfhost.New(repoCreds, anthCreds, bootstrap, adminHash)
	if err := edition.Bootstrap(ctx); err != nil {
		log.Fatalf("edition.Bootstrap: %v", err)
	}

	// Use cases
	enqueue := &usecase.EnqueueJob{
		Jobs:                 jobsRepo,
		RepoCreds:            repoCreds,
		AnthropicCreds:       anthCreds,
		Runner:               runner,
		IDGen:                idg,
		Clock:                clk,
		MaxConcurrentPerUser: cfg.MaxConcurrentPerUser,
		MaxConcurrentGlobal:  cfg.MaxConcurrentGlobal,
		OnComplete:           func(res ports.RunnerResult) { /* wired below */ },
	}
	getJob := &usecase.GetJob{Jobs: jobsRepo}
	listJobs := &usecase.ListJobs{Jobs: jobsRepo}
	complete := &usecase.HandleRunnerCompletion{Jobs: jobsRepo, Clock: clk}
	reattach := &usecase.ReattachRunningJobs{Jobs: jobsRepo, Runner: runner, Clock: clk}
	mint := &usecase.MintAPIKey{Keys: apiKeys, IDGen: idg, Clock: clk}
	revoke := &usecase.RevokeAPIKey{Keys: apiKeys}
	setAnth := &usecase.SetAnthropicCredentials{Secrets: secrets}
	dispatch := &usecase.DispatchCompletionWebhook{Dispatcher: hooks}

	enqueue.OnComplete = func(res ports.RunnerResult) {
		_ = complete.Execute(ctx, res)
		_ = dispatch // dispatch wired in once notification_webhook plumbing lands (Phase 2)
	}

	if err := reattach.Execute(ctx); err != nil {
		log.Printf("reattach (best effort): %v", err)
	}

	// HTTP
	jobsHandler := adhttp.NewJobsHandler(enqueue, getJob, listJobs)
	settingsHandler := adhttp.NewSettingsHandler(setAnth, mint, revoke)
	statusPage := adhttp.NewStatusPage(getJob)
	dashHandler := adhttp.NewDashboardHandler(listJobs, apiKeys, secrets, edition.Name(), editionResolver{e: edition})

	router := adhttp.NewRouter(adhttp.Deps{
		Resolver:        editionResolver{e: edition},
		JobsHandler:     jobsHandler,
		SettingsHandler: settingsHandler,
		StatusPage:      statusPage,
		Dashboard:       dashHandler,
		Edition:         edition,
	})

	log.Printf("listening on http://%s", cfg.HTTPBind)
	if err := http.ListenAndServe(cfg.HTTPBind, router); err != nil {
		log.Fatal(err)
	}
}

// editionResolver bridges runtime.Edition's ResolveUser into the HTTP
// adapter's UserResolver shape.
type editionResolver struct{ e *selfhost.Edition }

func (er editionResolver) Resolve(r *http.Request) (domain.UserID, error) {
	return er.e.ResolveUser(r)
}

// ----- reset-key -----

func runResetKey() {
	// Same as init's key-generation step, but doesn't touch users/PAT.
	cfg, err := config.Load()
	must("config.Load", err)
	db, err := postgres.Open(cfg.DSN)
	must("postgres.Open", err)
	defer db.Close()

	ctx := context.Background()
	keys := postgres.NewAPIKeysRepo(db)
	if existing, err := keys.ListForUser(ctx, selfhost.AdminUserID); err == nil {
		for _, k := range existing {
			_ = keys.Delete(ctx, k.ID, selfhost.AdminUserID)
		}
	}
	plain := newAdminKeyPlaintext()
	hash, _ := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	id := idgen.NanoID{}.NewAPIKeyID()
	prefix := plain[:8]
	_ = keys.Create(ctx, domain.NewAPIKey(domain.APIKeyID(id), selfhost.AdminUserID, "admin", prefix, domain.APIKeyHash(hash), time.Now().UTC()))
	fmt.Println("new admin API key:")
	fmt.Println(plain)
}

// ----- helpers -----

func must(what string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}

func loadAdminKeyHash(ctx context.Context, keys ports.APIKeysRepository) []byte {
	list, err := keys.ListForUser(ctx, selfhost.AdminUserID)
	if err != nil || len(list) == 0 {
		return nil
	}
	return []byte(list[0].Hash)
}

// encryptingSecrets is a tiny decorator that wraps SecretsRepository with
// AES-GCM at the composition seam. The Postgres impl stores bytes; this
// wrapper encrypts on write and decrypts on read.
type encryptingSecrets struct {
	inner ports.SecretsRepository
	aes   *crypto.AESGCM
}

// Compile-time assertion: encryptingSecrets satisfies ports.SecretsRepository.
var _ ports.SecretsRepository = (*encryptingSecrets)(nil)

func newEncryptingSecrets(inner ports.SecretsRepository, aes *crypto.AESGCM) *encryptingSecrets {
	return &encryptingSecrets{inner: inner, aes: aes}
}

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
