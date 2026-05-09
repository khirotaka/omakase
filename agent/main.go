package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("agent started",
		"poll_interval_sec", cfg.PollIntervalSec,
		"max_iteration", cfg.MaxIteration,
	)

	redis := NewRedisClient(cfg.UpstashRedisURL, cfg.UpstashRedisToken, nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("agent shutting down")
			return
		case <-ticker.C:
			if err := poll(ctx, redis); err != nil {
				slog.Error("poll error", "error", err)
			}
		}
	}
}

func poll(ctx context.Context, redis *RedisClient) error {
	slog.Info("polling queue", "queue", "agent-queue")

	raw, err := redis.RPOP(ctx, "agent-queue")
	if err != nil {
		return fmt.Errorf("poll: %w", err)
	}

	if raw == "" {
		slog.Info("queue empty")
		return nil
	}

	task, err := parseTask(raw)
	if err != nil {
		return fmt.Errorf("poll: %w", err)
	}

	slog.Info("task dequeued",
		"type", task.Type,
		"issue_number", task.IssueNumber,
		"repo_owner", task.RepoOwner,
		"repo_name", task.RepoName,
	)

	switch task.Type {
	case TaskTypeIssue:
		sess, err := CreateSession(ctx, redis, task.IssueNumber)
		if err != nil {
			return fmt.Errorf("poll: create session: %w", err)
		}
		slog.Info("session created",
			"issue_number", sess.IssueNumber,
			"status", sess.Status,
			"branch", sess.BranchName,
		)
		// TODO: dispatch to orchestrator for implementation (later feature)
	case TaskTypeReview:
		return fmt.Errorf("poll: review task handling not yet implemented")
	default:
		return fmt.Errorf("poll: unsupported task type %q", task.Type)
	}

	return nil
}
