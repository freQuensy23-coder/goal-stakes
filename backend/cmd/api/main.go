package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goalstakes/internal/ai"
	"goalstakes/internal/api"
	"goalstakes/internal/auth"
	"goalstakes/internal/config"
	"goalstakes/internal/scheduler"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
	chainweb3 "goalstakes/internal/web3"
	"goalstakes/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if err := run(); err != nil {
		log.Printf("api: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := applyMigrations(ctx, cfg.DatabaseURL); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	st := store.NewPostgres(pool)
	var charger service.PenaltyCharger = chainweb3.DisabledCharger{Reason: "web3: ENFORCER_PRIVATE_KEY is not configured"}
	var approvalChecker service.ApprovalChecker
	if cfg.EnforcerPrivateKey != "" {
		enforcer, err := chainweb3.NewEnforcer(ctx, cfg.Chains, cfg.EnforcerPrivateKey)
		if err != nil {
			return err
		}
		defer enforcer.Close()
		charger = enforcer
		approvalChecker = enforcer
	}
	serviceOptions := []service.Option{service.WithPenaltyCharger(charger)}
	if approvalChecker != nil {
		serviceOptions = append(serviceOptions, service.WithApprovalChecker(approvalChecker))
	}
	svc, err := service.New(st, cfg.Chains, serviceOptions...)
	if err != nil {
		return err
	}
	missedScheduler := scheduler.New(st, svc)
	go missedScheduler.Start(ctx, cfg.SchedulerInterval)
	siweAuth, err := auth.NewSIWEManager(st, cfg.JWTSecret, cfg.SIWEDomain, cfg.SessionTTL)
	if err != nil {
		return err
	}
	aiManager, err := ai.NewManager(st, svc, cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAITranscriptionModel, cfg.OpenAIBaseURL)
	if err != nil {
		return err
	}
	router := api.NewRouter(api.Config{
		Service:           svc,
		Sessions:          siweAuth,
		APIKeys:           auth.NewAPIKeyManager(st),
		SIWE:              siweAuth,
		AI:                aiManager,
		TelegramBotSecret: cfg.TelegramBotSecret,
		PublicBaseURL:     cfg.PublicBaseURL,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("api: listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func applyMigrations(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(db, "."); err != nil {
		return err
	}
	return nil
}
