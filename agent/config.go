package main

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AnthropicAPIKey   string
	UpstashRedisURL   string
	UpstashRedisToken string
	GitHubToken       string
	PollIntervalSec   int
	MaxIteration      int
}

var requiredEnvVars = []string{
	"ANTHROPIC_API_KEY",
	"UPSTASH_REDIS_URL",
	"UPSTASH_REDIS_TOKEN",
	"GITHUB_TOKEN",
}

func loadConfig() (*Config, error) {
	for _, key := range requiredEnvVars {
		if os.Getenv(key) == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", key)
		}
	}

	return &Config{
		AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
		UpstashRedisURL:   os.Getenv("UPSTASH_REDIS_URL"),
		UpstashRedisToken: os.Getenv("UPSTASH_REDIS_TOKEN"),
		GitHubToken:       os.Getenv("GITHUB_TOKEN"),
		PollIntervalSec:   getEnvInt("POLL_INTERVAL_SEC", 30),
		MaxIteration:      getEnvInt("MAX_ITERATION", 5),
	}, nil
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}
