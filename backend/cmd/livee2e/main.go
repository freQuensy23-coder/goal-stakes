package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
	chainweb3 "goalstakes/internal/web3"
	"goalstakes/migrations"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const confirmRealBurn = "burn-real-funds"

func main() {
	if err := run(); err != nil {
		log.Printf("livee2e: %v", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("LIVE_E2E_CONFIRM") != confirmRealBurn {
		return fmt.Errorf("LIVE_E2E_CONFIRM must be %q because this command burns real user funds", confirmRealBurn)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.EnforcerPrivateKey == "" {
		return errors.New("ENFORCER_PRIVATE_KEY is required for live e2e")
	}
	if cfg.AllowDisabledEnforcer {
		return errors.New("ALLOW_DISABLED_ENFORCER must be unset for live e2e")
	}

	live, err := liveConfig(cfg)
	if err != nil {
		return err
	}

	timeout := 5 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("LIVE_E2E_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("LIVE_E2E_TIMEOUT must be a positive Go duration")
		}
		timeout = parsed
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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

	enforcer, err := chainweb3.NewEnforcer(ctx, cfg.Chains, cfg.EnforcerPrivateKey)
	if err != nil {
		return err
	}
	defer enforcer.Close()
	svc, err := service.New(st, cfg.Chains, service.WithPenaltyCharger(enforcer), service.WithApprovalChecker(enforcer))
	if err != nil {
		return err
	}

	allowance, err := enforcer.AllowanceOf(ctx, live.Chain, live.UserWallet, live.TokenAddress)
	if err != nil {
		return err
	}
	if allowance.Cmp(live.Amount) < 0 {
		return fmt.Errorf("live allowance is %s, below LIVE_E2E_AMOUNT %s; approve the StakeEnforcer in MetaMask first", allowance.String(), live.Amount.String())
	}

	user, err := userForWallet(ctx, st, live.UserWallet)
	if err != nil {
		return err
	}
	goal, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       fmt.Sprintf("LIVE E2E burn %s", time.Now().UTC().Format(time.RFC3339)),
		Description: "Created by backend/cmd/livee2e after explicit operator confirmation.",
		Type:        domain.GoalAvoid,
		Cadence:     domain.CadenceDaily,
		StakeAmount: live.Amount.String(),
		TokenSymbol: live.TokenSymbol,
		Chain:       live.Chain,
		Timezone:    "UTC",
	})
	if err != nil {
		return err
	}
	violation, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "live e2e explicit burn test"})
	if err != nil {
		return err
	}
	if violation.Status != domain.ViolationCharged || strings.TrimSpace(violation.TxHash) == "" {
		return fmt.Errorf("violation status=%s tx=%q, want charged with tx hash", violation.Status, violation.TxHash)
	}

	log.Printf("livee2e: charged goal_id=%s violation_id=%s chain=%s token=%s amount=%s tx=%s", goal.ID, violation.ID, live.Chain, live.TokenSymbol, live.Amount.String(), violation.TxHash)
	return nil
}

type liveE2EConfig struct {
	UserWallet   string
	Chain        string
	TokenSymbol  string
	TokenAddress string
	Amount       *big.Int
}

func liveConfig(cfg config.Config) (liveE2EConfig, error) {
	wallet := strings.TrimSpace(os.Getenv("LIVE_E2E_USER_WALLET"))
	if !common.IsHexAddress(wallet) {
		return liveE2EConfig{}, fmt.Errorf("LIVE_E2E_USER_WALLET must be a 20-byte EVM address")
	}
	wallet = common.HexToAddress(wallet).Hex()
	chain := strings.TrimSpace(os.Getenv("LIVE_E2E_CHAIN"))
	chainCfg, ok := cfg.Chains[chain]
	if !ok {
		return liveE2EConfig{}, fmt.Errorf("LIVE_E2E_CHAIN %q is not present in CHAINS_JSON", chain)
	}
	token := strings.ToUpper(strings.TrimSpace(os.Getenv("LIVE_E2E_TOKEN_SYMBOL")))
	tokenAddress, ok := chainCfg.Tokens[token]
	if !ok {
		return liveE2EConfig{}, fmt.Errorf("LIVE_E2E_TOKEN_SYMBOL %q is not configured for chain %q", token, chain)
	}
	if !common.IsHexAddress(tokenAddress) {
		return liveE2EConfig{}, fmt.Errorf("configured token address for %s/%s is invalid", chain, token)
	}
	amount, err := parsePositiveIntEnv("LIVE_E2E_AMOUNT")
	if err != nil {
		return liveE2EConfig{}, err
	}
	maxRaw := strings.TrimSpace(os.Getenv("LIVE_E2E_MAX_AMOUNT"))
	if maxRaw == "" {
		maxRaw = "10000"
	}
	max, ok := new(big.Int).SetString(maxRaw, 10)
	if !ok || max.Sign() <= 0 {
		return liveE2EConfig{}, fmt.Errorf("LIVE_E2E_MAX_AMOUNT must be a positive integer")
	}
	if amount.Cmp(max) > 0 {
		return liveE2EConfig{}, fmt.Errorf("LIVE_E2E_AMOUNT %s is above LIVE_E2E_MAX_AMOUNT %s", amount.String(), max.String())
	}
	return liveE2EConfig{UserWallet: wallet, Chain: chain, TokenSymbol: token, TokenAddress: common.HexToAddress(tokenAddress).Hex(), Amount: amount}, nil
}

func parsePositiveIntEnv(name string) (*big.Int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer in token base units", name)
	}
	return value, nil
}

func userForWallet(ctx context.Context, st store.Store, wallet string) (domain.User, error) {
	user, err := st.GetUserByWallet(ctx, wallet)
	if errors.Is(err, store.ErrNotFound) {
		return st.CreateUser(ctx, wallet, "UTC")
	}
	return user, err
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
