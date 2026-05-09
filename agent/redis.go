package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const redisRequestTimeout = 10 * time.Second

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

// SET stores value at key with an optional TTL (pass 0 for no expiry).
// Uses the Upstash command-in-body format to safely handle JSON values.
func (r *RedisClient) SET(ctx context.Context, key, value string, ttl time.Duration) error {
	var cmd []any
	if ttl > 0 {
		cmd = []any{"SET", key, value, "EX", int64(ttl.Seconds())}
	} else {
		cmd = []any{"SET", key, value}
	}

	body, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("redis SET: marshal command: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, redisRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, r.baseURL, bytes.NewReader(body)) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
	if err != nil {
		return fmt.Errorf("redis SET: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
	if err != nil {
		return fmt.Errorf("redis SET: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("redis SET: unexpected status %d: %s", resp.StatusCode, b)
	}
	return nil
}

// GET returns the value stored at key, or "" if the key does not exist.
func (r *RedisClient) GET(ctx context.Context, key string) (string, error) {
	url := fmt.Sprintf("%s/get/%s", r.baseURL, url.PathEscape(key))

	reqCtx, cancel := context.WithTimeout(ctx, redisRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
	if err != nil {
		return "", fmt.Errorf("redis GET: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.httpClient.Do(req) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
	if err != nil {
		return "", fmt.Errorf("redis GET: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("redis GET: unexpected status %d: %s", resp.StatusCode, b)
	}

	var result redisResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("redis GET: decode response: %w", err)
	}

	if result.Result == nil {
		return "", nil
	}
	return *result.Result, nil
}

// RPOP removes and returns the last element of the list at key.
// Returns ("", nil) when the queue is empty.
func (r *RedisClient) RPOP(ctx context.Context, key string) (string, error) {
	url := fmt.Sprintf("%s/rpop/%s", r.baseURL, url.PathEscape(key))

	reqCtx, cancel := context.WithTimeout(ctx, redisRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, nil) //nolint:gosec // URL is from trusted config (UPSTASH_REDIS_URL)
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
