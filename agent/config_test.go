package main

import (
	"strings"
	"testing"
)

func setRequiredEnvVars(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("UPSTASH_REDIS_URL", "https://redis.example.com")
	t.Setenv("UPSTASH_REDIS_TOKEN", "test-redis-token")
	t.Setenv("GITHUB_TOKEN", "test-github-token")
	t.Setenv("SANDBOX_TEMPLATE", "test-sandbox-template")
}

func TestLoadConfig_AllRequiredVarsSet(t *testing.T) {
	setRequiredEnvVars(t)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.AnthropicAPIKey != "test-anthropic-key" {
		t.Errorf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "test-anthropic-key")
	}
	if cfg.UpstashRedisURL != "https://redis.example.com" {
		t.Errorf("UpstashRedisURL = %q, want %q", cfg.UpstashRedisURL, "https://redis.example.com")
	}
	if cfg.UpstashRedisToken != "test-redis-token" {
		t.Errorf("UpstashRedisToken = %q, want %q", cfg.UpstashRedisToken, "test-redis-token")
	}
	if cfg.GitHubToken != "test-github-token" {
		t.Errorf("GitHubToken = %q, want %q", cfg.GitHubToken, "test-github-token")
	}
	if cfg.SandboxTemplate != "test-sandbox-template" {
		t.Errorf("SandboxTemplate = %q, want %q", cfg.SandboxTemplate, "test-sandbox-template")
	}
	if cfg.SandboxNamespace != "default" {
		t.Errorf("SandboxNamespace = %q, want %q", cfg.SandboxNamespace, "default")
	}
}

func TestLoadConfig_MissingRequiredVar(t *testing.T) {
	tests := []struct {
		missingKey string
	}{
		{"ANTHROPIC_API_KEY"},
		{"UPSTASH_REDIS_URL"},
		{"UPSTASH_REDIS_TOKEN"},
		{"GITHUB_TOKEN"},
		{"SANDBOX_TEMPLATE"},
	}

	for _, tt := range tests {
		t.Run(tt.missingKey, func(t *testing.T) {
			setRequiredEnvVars(t)
			t.Setenv(tt.missingKey, "")

			_, err := loadConfig()
			if err == nil {
				t.Fatalf("expected error when %s is missing, got nil", tt.missingKey)
			}
			if !strings.Contains(err.Error(), tt.missingKey) {
				t.Errorf("error message %q does not contain variable name %q", err.Error(), tt.missingKey)
			}
		})
	}
}

func TestLoadConfig_PollIntervalDefault(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("POLL_INTERVAL_SEC", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalSec != 30 {
		t.Errorf("PollIntervalSec = %d, want 30", cfg.PollIntervalSec)
	}
}

func TestLoadConfig_PollIntervalInvalidValue(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("POLL_INTERVAL_SEC", "not-a-number")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalSec != 30 {
		t.Errorf("PollIntervalSec = %d, want default 30", cfg.PollIntervalSec)
	}
}

func TestLoadConfig_MaxIterationDefault(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("MAX_ITERATION", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxIteration != 5 {
		t.Errorf("MaxIteration = %d, want 5", cfg.MaxIteration)
	}
}

func TestLoadConfig_CustomOptionalVars(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("POLL_INTERVAL_SEC", "10")
	t.Setenv("MAX_ITERATION", "3")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalSec != 10 {
		t.Errorf("PollIntervalSec = %d, want 10", cfg.PollIntervalSec)
	}
	if cfg.MaxIteration != 3 {
		t.Errorf("MaxIteration = %d, want 3", cfg.MaxIteration)
	}
}
