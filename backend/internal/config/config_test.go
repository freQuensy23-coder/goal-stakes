package config

import (
	"strings"
	"testing"
	"time"
)

// validChainsJSON is a minimal, well-formed CHAINS_JSON covering the two
// testnet-first chains (PC1) with only the AS6-allowed tokens.
const validChainsJSON = `{
  "sepolia": {
    "rpc_url": "https://sepolia.example/rpc",
    "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
    "tokens": {
      "USDC": "0x2222222222222222222222222222222222222222",
      "USDT": "0x3333333333333333333333333333333333333333"
    }
  },
  "polygon-amoy": {
    "rpc_url": "https://amoy.example/rpc",
    "stake_enforcer_address": "0x4444444444444444444444444444444444444444",
    "tokens": {
      "USDC": "0x5555555555555555555555555555555555555555"
    }
  }
}`

// setRequired sets every REQUIRED env var to a valid value via t.Setenv, and
// clears the optional ones, so individual tests can then mutate one variable to
// exercise a specific failure. t.Setenv restores the environment after the test.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("HTTP_PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("JWT_SECRET", "test-jwt-secret")
	t.Setenv("SIWE_DOMAIN", "localhost:5173")
	t.Setenv("SESSION_TTL", "24h")
	t.Setenv("SCHEDULER_INTERVAL", "1m")
	t.Setenv("CHAINS_JSON", validChainsJSON)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_TRANSCRIPTION_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("ENFORCER_PRIVATE_KEY", "")
	t.Setenv("ALLOW_DISABLED_ENFORCER", "")
	t.Setenv("TELEGRAM_BOT_SECRET", "")
	t.Setenv("PUBLIC_BASE_URL", "")
}

func TestLoad_Valid(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if cfg.HTTPPort != "8080" {
		t.Errorf("HTTPPort = %q, want 8080", cfg.HTTPPort)
	}
	if cfg.DatabaseURL != "postgres://u:p@localhost:5432/db?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != "test-jwt-secret" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.SIWEDomain != "localhost:5173" {
		t.Errorf("SIWEDomain = %q", cfg.SIWEDomain)
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL = %v, want 24h", cfg.SessionTTL)
	}
	if cfg.SchedulerInterval != time.Minute {
		t.Errorf("SchedulerInterval = %v, want 1m", cfg.SchedulerInterval)
	}
	// AS3: OpenAI key is optional; empty is allowed and must not error.
	if cfg.OpenAIAPIKey != "" {
		t.Errorf("OpenAIAPIKey = %q, want empty", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModel != "" {
		t.Errorf("OpenAIModel = %q, want empty", cfg.OpenAIModel)
	}
	if cfg.OpenAITranscriptionModel != "" {
		t.Errorf("OpenAITranscriptionModel = %q, want empty", cfg.OpenAITranscriptionModel)
	}
	if cfg.EnforcerPrivateKey != "" {
		t.Errorf("EnforcerPrivateKey = %q, want empty", cfg.EnforcerPrivateKey)
	}
	if cfg.TelegramBotSecret != "" {
		t.Errorf("TelegramBotSecret = %q, want empty", cfg.TelegramBotSecret)
	}

	if len(cfg.Chains) != 2 {
		t.Fatalf("Chains: got %d chains, want 2: %+v", len(cfg.Chains), cfg.Chains)
	}

	sep, ok := cfg.Chains["sepolia"]
	if !ok {
		t.Fatal("Chains missing 'sepolia'")
	}
	if sep.RPCURL != "https://sepolia.example/rpc" {
		t.Errorf("sepolia RPCURL = %q", sep.RPCURL)
	}
	if sep.StakeEnforcerAddress != "0x1111111111111111111111111111111111111111" {
		t.Errorf("sepolia StakeEnforcerAddress = %q", sep.StakeEnforcerAddress)
	}
	// AS6: only the allow-listed tokens, with their contract addresses.
	if len(sep.Tokens) != 2 {
		t.Fatalf("sepolia tokens = %+v, want 2", sep.Tokens)
	}
	if got := sep.Tokens["USDC"]; got != "0x2222222222222222222222222222222222222222" {
		t.Errorf("sepolia USDC = %q", got)
	}
	if got := sep.Tokens["USDT"]; got != "0x3333333333333333333333333333333333333333" {
		t.Errorf("sepolia USDT = %q", got)
	}

	amoy, ok := cfg.Chains["polygon-amoy"]
	if !ok {
		t.Fatal("Chains missing 'polygon-amoy'")
	}
	if len(amoy.Tokens) != 1 || amoy.Tokens["USDC"] == "" {
		t.Errorf("polygon-amoy tokens = %+v, want just USDC", amoy.Tokens)
	}
}

// TestLoad_OptionalSet verifies the optional secrets are read when present.
func TestLoad_OptionalSet(t *testing.T) {
	setRequired(t)
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	t.Setenv("OPENAI_TRANSCRIPTION_MODEL", "whisper-test")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:9999/v1")
	t.Setenv("ENFORCER_PRIVATE_KEY", "0xprivkey")
	t.Setenv("TELEGRAM_BOT_SECRET", "bot-secret")
	t.Setenv("PUBLIC_BASE_URL", "https://api.goalstakes.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.OpenAIAPIKey != "sk-test" {
		t.Errorf("OpenAIAPIKey = %q, want sk-test", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModel != "gpt-test" {
		t.Errorf("OpenAIModel = %q, want gpt-test", cfg.OpenAIModel)
	}
	if cfg.OpenAITranscriptionModel != "whisper-test" {
		t.Errorf("OpenAITranscriptionModel = %q, want whisper-test", cfg.OpenAITranscriptionModel)
	}
	if cfg.OpenAIBaseURL != "http://127.0.0.1:9999/v1" {
		t.Errorf("OpenAIBaseURL = %q, want local compatible endpoint", cfg.OpenAIBaseURL)
	}
	if cfg.EnforcerPrivateKey != "0xprivkey" {
		t.Errorf("EnforcerPrivateKey = %q, want 0xprivkey", cfg.EnforcerPrivateKey)
	}
	if cfg.TelegramBotSecret != "bot-secret" {
		t.Errorf("TelegramBotSecret = %q, want bot-secret", cfg.TelegramBotSecret)
	}
	if cfg.PublicBaseURL != "https://api.goalstakes.example" {
		t.Errorf("PublicBaseURL = %q, want https://api.goalstakes.example", cfg.PublicBaseURL)
	}
}

func TestLoad_OpenAIRequiresTranscriptionModel(t *testing.T) {
	setRequired(t)
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	t.Setenv("OPENAI_TRANSCRIPTION_MODEL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected missing OPENAI_TRANSCRIPTION_MODEL error, got nil")
	} else if !strings.Contains(err.Error(), "OPENAI_TRANSCRIPTION_MODEL") {
		t.Fatalf("Load error %q should mention OPENAI_TRANSCRIPTION_MODEL", err)
	}
}

func TestLoad_InvalidOpenAIBaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	t.Setenv("OPENAI_TRANSCRIPTION_MODEL", "whisper-test")
	t.Setenv("OPENAI_BASE_URL", "not a url")

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected invalid OPENAI_BASE_URL error, got nil")
	} else if !strings.Contains(err.Error(), "OPENAI_BASE_URL") {
		t.Fatalf("Load error %q should mention OPENAI_BASE_URL", err)
	}
}

func TestLoad_InvalidPublicBaseURL(t *testing.T) {
	setRequired(t)
	t.Setenv("PUBLIC_BASE_URL", "not a url")

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected invalid PUBLIC_BASE_URL error, got nil")
	} else if !strings.Contains(err.Error(), "PUBLIC_BASE_URL") {
		t.Fatalf("Load error %q should mention PUBLIC_BASE_URL", err)
	}
}

// TestLoad_MissingRequired asserts fail-fast (GPC6) for each required variable:
// unsetting any one of them must produce an error naming it, never a silent
// default.
func TestLoad_MissingRequired(t *testing.T) {
	required := []string{"HTTP_PORT", "DATABASE_URL", "JWT_SECRET", "SIWE_DOMAIN", "SESSION_TTL", "SCHEDULER_INTERVAL", "CHAINS_JSON"}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(name, "")

			_, err := Load()
			if err == nil {
				t.Fatalf("Load: expected error when %s is empty, got nil", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("Load error %q does not mention missing var %s", err, name)
			}
		})
	}
}

// TestLoad_BadChainsJSON asserts a malformed CHAINS_JSON fails fast rather than
// loading a partial/empty chain map (GPC6).
func TestLoad_BadChainsJSON(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAINS_JSON", "{not valid json")

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected error for malformed CHAINS_JSON, got nil")
	}
}

// TestLoad_EmptyChainsMap rejects a syntactically valid but empty chain map: a
// backend with zero chains cannot enforce anything (GPC6).
func TestLoad_EmptyChainsMap(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAINS_JSON", "{}")

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected error for empty chains map, got nil")
	}
}

// TestLoad_DisallowedToken enforces AS6: a chain may only allow-list USDC/USDT.
// Any other token symbol must be rejected.
func TestLoad_DisallowedToken(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAINS_JSON", `{
      "sepolia": {
        "rpc_url": "https://sepolia.example/rpc",
        "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
        "tokens": { "DAI": "0x9999999999999999999999999999999999999999" }
      }
    }`)

	_, err := Load()
	if err == nil {
		t.Fatal("Load: expected error for disallowed token DAI (AS6), got nil")
	}
	if !strings.Contains(err.Error(), "DAI") {
		t.Fatalf("Load error %q should name the disallowed token", err)
	}
}

// TestLoad_MissingChainField rejects a chain entry that omits its RPC URL or
// contract address: those cannot be defaulted (GPC1).
func TestLoad_MissingChainField(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAINS_JSON", `{
      "sepolia": {
        "rpc_url": "",
        "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
        "tokens": { "USDC": "0x2222222222222222222222222222222222222222" }
      }
    }`)

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected error for empty rpc_url, got nil")
	}
}

func TestLoad_InvalidChainAddress(t *testing.T) {
	tests := []struct {
		name       string
		chainsJSON string
		want       string
	}{
		{
			name: "enforcer",
			chainsJSON: `{
              "sepolia": {
                "rpc_url": "https://sepolia.example/rpc",
                "stake_enforcer_address": "not-an-address",
                "tokens": { "USDC": "0x2222222222222222222222222222222222222222" }
              }
            }`,
			want: "stake_enforcer_address",
		},
		{
			name: "token",
			chainsJSON: `{
              "sepolia": {
                "rpc_url": "https://sepolia.example/rpc",
                "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
                "tokens": { "USDC": "not-an-address" }
              }
            }`,
			want: "USDC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CHAINS_JSON", tt.chainsJSON)

			_, err := Load()
			if err == nil {
				t.Fatal("Load: expected invalid address error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestLoad_RejectsZeroAddresses(t *testing.T) {
	tests := []struct {
		name       string
		chainsJSON string
		want       string
	}{
		{
			name: "enforcer",
			chainsJSON: `{
              "sepolia": {
                "rpc_url": "https://sepolia.example/rpc",
                "stake_enforcer_address": "0x0000000000000000000000000000000000000000",
                "tokens": { "USDC": "0x2222222222222222222222222222222222222222" }
              }
            }`,
			want: "stake_enforcer_address",
		},
		{
			name: "token",
			chainsJSON: `{
              "sepolia": {
                "rpc_url": "https://sepolia.example/rpc",
                "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
                "tokens": { "USDC": "0x0000000000000000000000000000000000000000" }
              }
            }`,
			want: "USDC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CHAINS_JSON", tt.chainsJSON)

			_, err := Load()
			if err == nil {
				t.Fatal("Load: expected zero address error, got nil")
			}
			if !strings.Contains(err.Error(), "zero address") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error %q should mention zero address and %q", err, tt.want)
			}
		})
	}
}

func TestLoad_InvalidRPCURL(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAINS_JSON", `{
      "sepolia": {
        "rpc_url": "not a url",
        "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
        "tokens": { "USDC": "0x2222222222222222222222222222222222222222" }
      }
    }`)

	_, err := Load()
	if err == nil {
		t.Fatal("Load: expected invalid rpc_url error, got nil")
	}
	if !strings.Contains(err.Error(), "rpc_url") {
		t.Fatalf("Load error %q should mention rpc_url", err)
	}
}

func TestLoad_MainnetChainsRequireCanonicalTokens(t *testing.T) {
	tests := []struct {
		name       string
		chainsJSON string
		want       string
	}{
		{
			name: "ethereum wrong USDC",
			chainsJSON: `{
              "ethereum": {
                "rpc_url": "https://mainnet.example/rpc",
                "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
                "tokens": {
                  "USDC": "0x2222222222222222222222222222222222222222",
                  "USDT": "0xdAC17F958D2ee523a2206206994597C13D831ec7"
                }
              }
            }`,
			want: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		},
		{
			name: "polygon wrong USDT",
			chainsJSON: `{
              "polygon": {
                "rpc_url": "https://polygon.example/rpc",
                "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
                "tokens": {
                  "USDC": "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
                  "USDT": "0x2222222222222222222222222222222222222222"
                }
              }
            }`,
			want: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F",
		},
		{
			name: "polygon missing USDT",
			chainsJSON: `{
              "polygon": {
                "rpc_url": "https://polygon.example/rpc",
                "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
                "tokens": {
                  "USDC": "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359"
                }
              }
            }`,
			want: "USDT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("CHAINS_JSON", tt.chainsJSON)

			_, err := Load()
			if err == nil {
				t.Fatal("Load: expected mainnet token validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestLoad_MainnetChainsAcceptCanonicalTokens(t *testing.T) {
	setRequired(t)
	t.Setenv("ENFORCER_PRIVATE_KEY", "0xabc123")
	t.Setenv("CHAINS_JSON", `{
      "ethereum": {
        "rpc_url": "https://mainnet.example/rpc",
        "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
        "tokens": {
          "USDC": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
          "USDT": "0xdAC17F958D2ee523a2206206994597C13D831ec7"
        }
      },
      "polygon": {
        "rpc_url": "https://polygon.example/rpc",
        "stake_enforcer_address": "0x2222222222222222222222222222222222222222",
        "tokens": {
          "USDC": "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
          "USDT": "0xc2132D05D31c914a87C6611C10748AEb04B58e8F"
        }
      }
    }`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: unexpected error for canonical mainnet tokens: %v", err)
	}
	if len(cfg.Chains["ethereum"].Tokens) != 2 || len(cfg.Chains["polygon"].Tokens) != 2 {
		t.Fatalf("mainnet token maps = %+v", cfg.Chains)
	}
}

func TestLoad_MainnetChainsRequireEnforcerKeyUnlessExplicitlyAllowed(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAINS_JSON", `{
      "ethereum": {
        "rpc_url": "https://mainnet.example/rpc",
        "stake_enforcer_address": "0x1111111111111111111111111111111111111111",
        "tokens": {
          "USDC": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
          "USDT": "0xdAC17F958D2ee523a2206206994597C13D831ec7"
        }
      }
    }`)

	_, err := Load()
	if err == nil {
		t.Fatal("Load: expected mainnet enforcer key error, got nil")
	}
	if !strings.Contains(err.Error(), "ENFORCER_PRIVATE_KEY") || !strings.Contains(err.Error(), "ALLOW_DISABLED_ENFORCER") {
		t.Fatalf("Load error %q should mention ENFORCER_PRIVATE_KEY and ALLOW_DISABLED_ENFORCER", err)
	}

	t.Setenv("ALLOW_DISABLED_ENFORCER", "true")
	if _, err := Load(); err != nil {
		t.Fatalf("Load with explicit disabled-enforcer allowance: %v", err)
	}
}

// TestDefaultChains documents the testnet-first chain set (PC1). It is exported
// for later phases and tests to reference instead of hardcoding names.
func TestDefaultChains(t *testing.T) {
	if len(DefaultChains) != 2 {
		t.Fatalf("DefaultChains = %v, want 2 testnet chains", DefaultChains)
	}
	want := map[string]bool{"sepolia": true, "polygon-amoy": true}
	for _, c := range DefaultChains {
		if !want[c] {
			t.Errorf("unexpected default chain %q", c)
		}
	}
}
