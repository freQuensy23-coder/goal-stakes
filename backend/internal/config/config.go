// Package config loads all runtime configuration from the process environment.
//
// Configuration policy (GPC1): every field is read from an explicit environment
// variable — there are no hidden defaults for ports, URLs, secrets, or chain
// parameters. Required variables that are absent or empty cause Load to fail
// fast with an error naming the variable (GPC6); nothing is silently defaulted.
//
// Two secrets are intentionally optional:
//   - OPENAI_API_KEY (AS3): the AI coach and transcription are optional; empty
//     disables them. OPENAI_BASE_URL can point at an OpenAI-compatible endpoint
//     for local e2e.
//   - ENFORCER_PRIVATE_KEY: empty disables real penalty transactions and live
//     allowance checks for local UI/API work.
//   - ALLOW_DISABLED_ENFORCER: when true, allows ethereum/polygon config without
//     ENFORCER_PRIVATE_KEY for local/dry-run verification only.
//
// Per-chain configuration is supplied through a single explicit variable,
// CHAINS_JSON, holding a JSON object keyed by chain name. Each entry carries
// the RPC URL, the StakeEnforcer contract address, and an allow-list of tokens
// (symbol -> ERC-20 contract address). The token allow-list enforces AS6: only
// USDC and USDT are permitted; any other symbol is rejected. RPC URLs and
// contract addresses are never hardcoded — they always come from CHAINS_JSON.
//
// Example CHAINS_JSON (testnet-first for local verification):
//
//	{
//	  "sepolia": {
//	    "rpc_url": "https://sepolia.infura.io/v3/<key>",
//	    "stake_enforcer_address": "0xabc...",
//	    "tokens": { "USDC": "0x...", "USDT": "0x..." }
//	  },
//	  "polygon-amoy": {
//	    "rpc_url": "https://rpc-amoy.polygon.technology",
//	    "stake_enforcer_address": "0xdef...",
//	    "tokens": { "USDC": "0x..." }
//	  }
//	}
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

var zeroAddress = common.Address{}

// DefaultChains is the canonical testnet-first chain set for local verification.
// Production deployments can use "ethereum" and "polygon" by providing them in
// CHAINS_JSON. This is NOT a source of RPC URLs or addresses — those must come
// from CHAINS_JSON (GPC1).
var DefaultChains = []string{"sepolia", "polygon-amoy"}

// allowedTokens is the AS6 allow-list: only these stablecoin symbols may appear
// in any chain's token map. Anything else is rejected at load time.
var allowedTokens = map[string]bool{"USDC": true, "USDT": true}

// canonicalMainnetTokens protects the chain keys that the web client maps to
// live Ethereum and Polygon networks. Addresses checked against Circle, Tether,
// and Polygon docs on 2026-06-30.
var canonicalMainnetTokens = map[string]map[string]string{
	"ethereum": {
		"USDC": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		"USDT": "0xdAC17F958D2ee523a2206206994597C13D831ec7",
	},
	"polygon": {
		"USDC": "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
		"USDT": "0xc2132D05D31c914a87C6611C10748AEb04B58e8F",
	},
}

// ChainConfig holds the per-chain parameters needed to detect approvals and
// execute charges via the StakeEnforcer contract.
type ChainConfig struct {
	// RPCURL is the JSON-RPC endpoint for this chain.
	RPCURL string `json:"rpc_url"`
	// StakeEnforcerAddress is the deployed StakeEnforcer contract address.
	StakeEnforcerAddress string `json:"stake_enforcer_address"`
	// Tokens maps an allow-listed token symbol (USDC/USDT, AS6) to its ERC-20
	// contract address on this chain.
	Tokens map[string]string `json:"tokens"`
}

// Config is the fully-resolved, validated runtime configuration. All fields are
// populated by Load from the environment.
type Config struct {
	// HTTPPort is the TCP port the API server listens on (HTTP_PORT). Required.
	HTTPPort string
	// DatabaseURL is the Postgres connection string (DATABASE_URL). Required.
	DatabaseURL string
	// JWTSecret signs/verifies session tokens (JWT_SECRET). Required.
	JWTSecret string
	// SIWEDomain is the EIP-4361 domain users sign for (SIWE_DOMAIN). Required.
	SIWEDomain string
	// SessionTTL controls session JWT expiration (SESSION_TTL). Required.
	SessionTTL time.Duration
	// SchedulerInterval controls missed-goal scans (SCHEDULER_INTERVAL). Required.
	SchedulerInterval time.Duration
	// OpenAIAPIKey enables the AI coach (OPENAI_API_KEY). Optional (AS3).
	OpenAIAPIKey string
	// OpenAIModel is required when OpenAIAPIKey is set (OPENAI_MODEL).
	OpenAIModel string
	// OpenAITranscriptionModel is required when OpenAIAPIKey is set
	// (OPENAI_TRANSCRIPTION_MODEL).
	OpenAITranscriptionModel string
	// OpenAIBaseURL overrides the OpenAI-compatible API root (OPENAI_BASE_URL).
	// Optional; empty uses the SDK default.
	OpenAIBaseURL string
	// EnforcerPrivateKey is the on-chain signer key (ENFORCER_PRIVATE_KEY).
	// Optional; empty disables real penalty transactions and live allowance reads.
	EnforcerPrivateKey string
	// TelegramBotSecret authenticates Telegram bot calls to /internal/telegram/*.
	// Optional until the bot is enabled.
	TelegramBotSecret string
	// PublicBaseURL is the externally visible backend origin used for private
	// generated links such as /agent-skills/{token}.md. Optional; requests can
	// still derive a local origin from Host headers.
	PublicBaseURL string
	// AllowDisabledEnforcer is a local/dry-run escape hatch for loading mainnet
	// chain keys without ENFORCER_PRIVATE_KEY. It must not be enabled for go-live.
	AllowDisabledEnforcer bool
	// Chains maps chain name -> ChainConfig, parsed from CHAINS_JSON. Required
	// and non-empty.
	Chains map[string]ChainConfig
}

// Load reads, parses, and validates configuration from the environment. It
// returns a descriptive error (GPC6) and a zero Config if any required variable
// is missing or any value is invalid; on success it returns a complete Config.
func Load() (Config, error) {
	var cfg Config
	var err error

	if cfg.HTTPPort, err = requireEnv("HTTP_PORT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseURL, err = requireEnv("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.JWTSecret, err = requireEnv("JWT_SECRET"); err != nil {
		return Config{}, err
	}
	if cfg.SIWEDomain, err = requireEnv("SIWE_DOMAIN"); err != nil {
		return Config{}, err
	}
	sessionTTL, err := requireEnv("SESSION_TTL")
	if err != nil {
		return Config{}, err
	}
	cfg.SessionTTL, err = time.ParseDuration(sessionTTL)
	if err != nil {
		return Config{}, fmt.Errorf("config: SESSION_TTL must be a positive Go duration: %w", err)
	}
	if cfg.SessionTTL <= 0 {
		return Config{}, fmt.Errorf("config: SESSION_TTL must be positive")
	}
	schedulerInterval, err := requireEnv("SCHEDULER_INTERVAL")
	if err != nil {
		return Config{}, err
	}
	cfg.SchedulerInterval, err = time.ParseDuration(schedulerInterval)
	if err != nil {
		return Config{}, fmt.Errorf("config: SCHEDULER_INTERVAL must be a positive Go duration: %w", err)
	}
	if cfg.SchedulerInterval <= 0 {
		return Config{}, fmt.Errorf("config: SCHEDULER_INTERVAL must be positive")
	}

	// Optional secrets: empty is a valid, meaningful value, not an error.
	cfg.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")                         // AS3: optional.
	cfg.OpenAIModel = os.Getenv("OPENAI_MODEL")                            // Required only when OPENAI_API_KEY is set.
	cfg.OpenAITranscriptionModel = os.Getenv("OPENAI_TRANSCRIPTION_MODEL") // Required only when OPENAI_API_KEY is set.
	cfg.OpenAIBaseURL = os.Getenv("OPENAI_BASE_URL")                       // Optional OpenAI-compatible endpoint.
	cfg.EnforcerPrivateKey = os.Getenv("ENFORCER_PRIVATE_KEY")
	cfg.TelegramBotSecret = os.Getenv("TELEGRAM_BOT_SECRET")
	cfg.PublicBaseURL = os.Getenv("PUBLIC_BASE_URL")
	if rawAllow := os.Getenv("ALLOW_DISABLED_ENFORCER"); rawAllow != "" {
		allow, err := strconv.ParseBool(rawAllow)
		if err != nil {
			return Config{}, fmt.Errorf("config: ALLOW_DISABLED_ENFORCER must be true or false: %w", err)
		}
		cfg.AllowDisabledEnforcer = allow
	}
	if cfg.OpenAIAPIKey != "" && cfg.OpenAIModel == "" {
		return Config{}, fmt.Errorf("config: OPENAI_MODEL is required when OPENAI_API_KEY is set")
	}
	if cfg.OpenAIAPIKey != "" && cfg.OpenAITranscriptionModel == "" {
		return Config{}, fmt.Errorf("config: OPENAI_TRANSCRIPTION_MODEL is required when OPENAI_API_KEY is set")
	}
	if cfg.OpenAIBaseURL != "" {
		if err := validateHTTPURL(cfg.OpenAIBaseURL); err != nil {
			return Config{}, fmt.Errorf("config: OPENAI_BASE_URL is invalid: %w", err)
		}
	}
	if cfg.PublicBaseURL != "" {
		if err := validateHTTPURL(cfg.PublicBaseURL); err != nil {
			return Config{}, fmt.Errorf("config: PUBLIC_BASE_URL is invalid: %w", err)
		}
	}

	chainsJSON, err := requireEnv("CHAINS_JSON")
	if err != nil {
		return Config{}, err
	}
	cfg.Chains, err = parseChains(chainsJSON)
	if err != nil {
		return Config{}, err
	}
	if cfg.EnforcerPrivateKey == "" && !cfg.AllowDisabledEnforcer && hasMainnetChains(cfg.Chains) {
		return Config{}, fmt.Errorf("config: ENFORCER_PRIVATE_KEY is required when CHAINS_JSON includes ethereum or polygon; set ALLOW_DISABLED_ENFORCER=true only for local dry-run verification")
	}

	return cfg, nil
}

func hasMainnetChains(chains map[string]ChainConfig) bool {
	_, hasEthereum := chains["ethereum"]
	_, hasPolygon := chains["polygon"]
	return hasEthereum || hasPolygon
}

// requireEnv returns the value of a required environment variable, or an error
// naming it if the variable is unset or empty (GPC6). No default is invented.
func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("config: required environment variable %s is not set", name)
	}
	return v, nil
}

// parseChains decodes and validates CHAINS_JSON. It enforces: valid JSON, a
// non-empty chain map, a valid RPC URL and nonzero StakeEnforcer address per
// chain, at least one token per chain, and that every token symbol is on the AS6
// allow-list with a nonzero contract address. For ethereum/polygon, the token
// addresses must be the canonical mainnet USDC/USDT contracts. DisallowUnknownFields
// rejects typo'd keys rather than silently dropping them (GPC6).
func parseChains(raw string) (map[string]ChainConfig, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()

	var chains map[string]ChainConfig
	if err := dec.Decode(&chains); err != nil {
		return nil, fmt.Errorf("config: CHAINS_JSON is not valid JSON: %w", err)
	}
	if len(chains) == 0 {
		return nil, fmt.Errorf("config: CHAINS_JSON must define at least one chain")
	}

	// Validate in sorted chain-name order so error messages are deterministic.
	names := make([]string, 0, len(chains))
	for name := range chains {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := chains[name]
		if name == "" {
			return nil, fmt.Errorf("config: CHAINS_JSON contains an empty chain name")
		}
		if c.RPCURL == "" {
			return nil, fmt.Errorf("config: chain %q is missing rpc_url", name)
		}
		if err := validateRPCURL(c.RPCURL); err != nil {
			return nil, fmt.Errorf("config: chain %q rpc_url is invalid: %w", name, err)
		}
		if c.StakeEnforcerAddress == "" {
			return nil, fmt.Errorf("config: chain %q is missing stake_enforcer_address", name)
		}
		if err := validateNonZeroAddress(c.StakeEnforcerAddress); err != nil {
			return nil, fmt.Errorf("config: chain %q stake_enforcer_address is not a valid EVM address: %w", name, err)
		}
		if len(c.Tokens) == 0 {
			return nil, fmt.Errorf("config: chain %q must allow-list at least one token", name)
		}
		for symbol, addr := range c.Tokens {
			if !allowedTokens[symbol] {
				return nil, fmt.Errorf("config: chain %q lists disallowed token %q (only USDC/USDT permitted, AS6)", name, symbol)
			}
			if addr == "" {
				return nil, fmt.Errorf("config: chain %q token %q is missing a contract address", name, symbol)
			}
			if err := validateNonZeroAddress(addr); err != nil {
				return nil, fmt.Errorf("config: chain %q token %q is not a valid EVM address: %w", name, symbol, err)
			}
		}
		if err := validateCanonicalMainnetTokens(name, c.Tokens); err != nil {
			return nil, err
		}
	}

	return chains, nil
}

func validateCanonicalMainnetTokens(chain string, tokens map[string]string) error {
	expected, ok := canonicalMainnetTokens[chain]
	if !ok {
		return nil
	}
	for symbol, expectedAddress := range expected {
		actual, ok := tokens[symbol]
		if !ok {
			return fmt.Errorf("config: chain %q must include canonical %s token %s", chain, symbol, expectedAddress)
		}
		if !strings.EqualFold(actual, expectedAddress) {
			return fmt.Errorf("config: chain %q token %q must use canonical mainnet address %s", chain, symbol, expectedAddress)
		}
	}
	return nil
}

func validateRPCURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
	default:
		return fmt.Errorf("scheme must be http, https, ws, or wss")
	}
	if u.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func validateNonZeroAddress(raw string) error {
	if !common.IsHexAddress(raw) {
		return fmt.Errorf("invalid EVM address")
	}
	if common.HexToAddress(raw) == zeroAddress {
		return fmt.Errorf("zero address")
	}
	return nil
}
