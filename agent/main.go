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
			if err := poll(ctx, cfg, redis); err != nil {
				slog.Error("poll error", "error", err)
			}
		}
	}
}

func poll(ctx context.Context, cfg *Config, redis *RedisClient) error {
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
		return handleIssueTask(ctx, cfg, redis, task)
	case TaskTypeReview:
		return handleReviewTask(ctx, cfg, redis, task)
	default:
		return fmt.Errorf("poll: unsupported task type %q", task.Type)
	}
}

func handleIssueTask(ctx context.Context, cfg *Config, redis *RedisClient, task *Task) error {
	_ = cfg
	sess, err := CreateSession(ctx, redis, task.IssueNumber)
	if err != nil {
		return fmt.Errorf("handleIssueTask: create session: %w", err)
	}
	slog.Info("session created",
		"issue_number", sess.IssueNumber,
		"status", sess.Status,
		"branch", sess.BranchName,
	)
	if err := RunIssueCycle(ctx, cfg, redis, task, sess); err != nil {
		return fmt.Errorf("handleIssueTask: %w", err)
	}
	return nil
}

func handleReviewTask(ctx context.Context, cfg *Config, redis *RedisClient, task *Task) error {
	sess, err := ReadSession(ctx, redis, task.IssueNumber)
	if err != nil {
		return fmt.Errorf("handleReviewTask: read session: %w", err)
	}
	if sess == nil {
		return fmt.Errorf("handleReviewTask: no session found for issue %d", task.IssueNumber)
	}

	if sess.Status != StatusReviewPending {
		return fmt.Errorf("handleReviewTask: invalid state %q for issue %d; expected %q",
			sess.Status, task.IssueNumber, StatusReviewPending)
	}

	ghClient := newGitHubClient(ctx, cfg.GitHubToken)

	if sess.Iteration >= cfg.MaxIteration {
		sess.Status = StatusAborted
		if uerr := UpdateSession(ctx, redis, sess); uerr != nil {
			return fmt.Errorf("handleReviewTask: abort session: %w", uerr)
		}
		msg := fmt.Sprintf(
			"Agent aborted: reached maximum iteration limit (%d). Please review and continue manually.",
			cfg.MaxIteration,
		)
		if cerr := PostIssueComment(ctx, ghClient, task.RepoOwner, task.RepoName, task.IssueNumber, msg); cerr != nil {
			slog.Error("failed to post abort comment", "error", cerr, "issue_number", task.IssueNumber)
		}
		slog.Info("session aborted: max iteration reached",
			"issue_number", task.IssueNumber,
			"iteration", sess.Iteration,
			"max_iteration", cfg.MaxIteration,
		)
		return nil
	}

	sess.Status = StatusFixing
	sess.Iteration++
	if err := UpdateSession(ctx, redis, sess); err != nil {
		return fmt.Errorf("handleReviewTask: update session to fixing: %w", err)
	}
	slog.Info("session transitioned to fixing",
		"issue_number", sess.IssueNumber,
		"iteration", sess.Iteration,
		"review_body_len", len(task.Body),
	)
	if err := RunFixCycle(ctx, cfg, redis, task, sess); err != nil {
		return fmt.Errorf("handleReviewTask: %w", err)
	}
	return nil
}
