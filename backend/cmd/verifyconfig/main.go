package main

import (
	"context"
	"log"
	"os"
	"sort"
	"time"

	"goalstakes/internal/config"
	chainweb3 "goalstakes/internal/web3"
)

func main() {
	if err := run(); err != nil {
		log.Printf("verifyconfig: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Chains))
	for name := range cfg.Chains {
		names = append(names, name)
	}
	sort.Strings(names)
	log.Printf("verifyconfig: loaded %d chain(s): %v", len(names), names)

	if cfg.EnforcerPrivateKey == "" {
		log.Printf("verifyconfig: ENFORCER_PRIVATE_KEY is empty; skipped live StakeEnforcer validation")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	enforcer, err := chainweb3.NewEnforcer(ctx, cfg.Chains, cfg.EnforcerPrivateKey)
	if err != nil {
		return err
	}
	defer enforcer.Close()
	log.Printf("verifyconfig: live StakeEnforcer validation passed")
	return nil
}
