package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RedisClient is a minimal Upstash Redis HTTP client.
type RedisClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewRedisClient creates a RedisClient from the given Upstash endpoint and token.
// Pass nil for client to use the default http.Client.
func NewRedisClient(baseURL, token string, client *http.Client) *RedisClient {
	if client == nil {
		client = &http.Client{}
	}
	return &RedisClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

type redisResponse struct {
	Result *string `json:"result"`
}

// RPOP removes and returns the last element of the list at key.
// Returns ("", nil) when the queue is empty.
func (r *RedisClient) RPOP(ctx context.Context, key string) (string, error) {
	url := fmt.Sprintf("%s/rpop/%s", r.baseURL, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
	if err != nil {
		return "", fmt.Errorf("redis RPOP: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.httpClient.Do(req) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
	if err != nil {
		return "", fmt.Errorf("redis RPOP: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("redis RPOP: unexpected status %d: %s", resp.StatusCode, body)
	}

	var result redisResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("redis RPOP: decode response: %w", err)
	}

	if result.Result == nil {
		return "", nil
	}
	return *result.Result, nil
}
