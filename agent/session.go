package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SessionStatus represents the lifecycle state of an AgentSession.
type SessionStatus string

const (
	StatusDeveloping    SessionStatus = "developing"
	StatusReviewPending SessionStatus = "review_pending"
	StatusFixing        SessionStatus = "fixing"
	StatusDone          SessionStatus = "done"
	StatusAborted       SessionStatus = "aborted"

	sessionTTL = 24 * time.Hour
)

// AgentSession tracks the state of an agent working on a GitHub issue.
type AgentSession struct {
	IssueNumber   int           `json:"issueNumber"`
	Status        SessionStatus `json:"status"`
	BranchName    string        `json:"branchName"`
	PRNumber      int           `json:"prNumber,omitempty"`
	Iteration     int           `json:"iteration"`
	GeneratedFile string        `json:"generatedFile,omitempty"` // repo-relative path of the LLM-generated file
}

func sessionKey(issueNumber int) string {
	return fmt.Sprintf("session:%d", issueNumber)
}

// CreateSession stores a new AgentSession in Redis with status=developing and TTL=24h.
// Returns an error if a session already exists for the issue.
func CreateSession(ctx context.Context, redis *RedisClient, issueNumber int) (*AgentSession, error) {
	existing, err := ReadSession(ctx, redis, issueNumber)
	if err != nil {
		return nil, fmt.Errorf("CreateSession: check existing: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("CreateSession: session already exists for issue %d", issueNumber)
	}

	sess := &AgentSession{
		IssueNumber: issueNumber,
		Status:      StatusDeveloping,
		BranchName:  fmt.Sprintf("agent/issue-%d", issueNumber),
		Iteration:   0,
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("CreateSession: marshal: %w", err)
	}

	key := sessionKey(issueNumber)
	if err := redis.SET(ctx, key, string(data), sessionTTL); err != nil {
		return nil, fmt.Errorf("CreateSession: %w", err)
	}

	return sess, nil
}

// ReadSession retrieves an AgentSession from Redis by issue number.
// Returns (nil, nil) when the session does not exist.
func ReadSession(ctx context.Context, redis *RedisClient, issueNumber int) (*AgentSession, error) {
	raw, err := redis.GET(ctx, sessionKey(issueNumber))
	if err != nil {
		return nil, fmt.Errorf("ReadSession: %w", err)
	}
	if raw == "" {
		return nil, nil
	}

	var sess AgentSession
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, fmt.Errorf("ReadSession: unmarshal: %w", err)
	}
	return &sess, nil
}

// UpdateSession persists an updated AgentSession back to Redis, resetting the TTL to 24h.
func UpdateSession(ctx context.Context, redis *RedisClient, sess *AgentSession) error {
	if sess == nil {
		return fmt.Errorf("UpdateSession: session is nil")
	}
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("UpdateSession: marshal: %w", err)
	}

	if err := redis.SET(ctx, sessionKey(sess.IssueNumber), string(data), sessionTTL); err != nil {
		return fmt.Errorf("UpdateSession: %w", err)
	}
	return nil
}
