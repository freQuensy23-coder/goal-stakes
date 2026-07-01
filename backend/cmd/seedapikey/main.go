package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"goalstakes/internal/config"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
	"goalstakes/migrations"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if err := run(); err != nil {
		log.Printf("seedapikey: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	wallet := strings.TrimSpace(os.Getenv("LIVE_E2E_USER_WALLET"))
	if !common.IsHexAddress(wallet) {
		return fmt.Errorf("LIVE_E2E_USER_WALLET must be a 20-byte EVM address")
	}
	wallet = common.HexToAddress(wallet).Hex()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
	user, err := st.GetUserByWallet(ctx, wallet)
	if errors.Is(err, store.ErrNotFound) {
		user, err = st.CreateUser(ctx, wallet, "UTC")
	}
	if err != nil {
		return err
	}
	svc, err := service.New(st, cfg.Chains)
	if err != nil {
		return err
	}
	created, err := svc.CreateAPIKey(ctx, user.ID, "live e2e")
	if err != nil {
		return err
	}
	fmt.Println(created.Raw)
	return nil
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
