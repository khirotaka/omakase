package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// RunIssueCycle implements the full workflow for a new GitHub issue:
// LLM plan → code generation → sandbox execution → PR creation → session transition.
func RunIssueCycle(ctx context.Context, cfg *Config, redis *RedisClient, task *Task, sess *AgentSession) error {
	slog.Info("running issue cycle", "issue_number", task.IssueNumber)

	plan, err := GenerateImplementationPlan(ctx, cfg.AnthropicAPIKey, task.Body)
	if err != nil {
		return fmt.Errorf("RunIssueCycle: generate plan: %w", err)
	}
	slog.Info("implementation plan generated", "issue_number", task.IssueNumber)

	filename, targetDir, code, err := GenerateCode(ctx, cfg.AnthropicAPIKey, task.Body, plan)
	if err != nil {
		return fmt.Errorf("RunIssueCycle: generate code: %w", err)
	}
	slog.Info("code generated", "filename", filename, "target_dir", targetDir)

	sb, err := NewSandbox(ctx, cfg.SandboxTemplate, cfg.SandboxNamespace)
	if err != nil {
		return fmt.Errorf("RunIssueCycle: %w", err)
	}
	defer func() {
		if cerr := sb.Close(ctx); cerr != nil {
			slog.Error("sandbox close failed", "error", cerr)
		}
	}()

	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git",
		cfg.GitHubToken, task.RepoOwner, task.RepoName)

	if out, err := sb.Run(ctx, fmt.Sprintf("git clone %s /workspace", cloneURL)); err != nil {
		return fmt.Errorf("RunIssueCycle: git clone: %w", err)
	} else {
		slog.Info("repo cloned", "output", out)
	}

	if _, err := sb.Run(ctx, "git config --global user.email 'agent@omakase.ai' && git config --global user.name 'Omakase Agent'"); err != nil {
		return fmt.Errorf("RunIssueCycle: git config: %w", err)
	}

	if out, err := sb.Run(ctx, fmt.Sprintf("cd /workspace && git checkout -b %s", sess.BranchName)); err != nil {
		return fmt.Errorf("RunIssueCycle: git checkout: %w", err)
	} else {
		slog.Info("branch created", "branch", sess.BranchName, "output", out)
	}

	if err := sb.Write(ctx, filename, []byte(code)); err != nil {
		return fmt.Errorf("RunIssueCycle: write file: %w", err)
	}

	targetDir = strings.TrimSuffix(targetDir, "/")
	destPath := fmt.Sprintf("/workspace/%s", strings.TrimPrefix(targetDir, "./"))
	if _, err := sb.Run(ctx, fmt.Sprintf("mkdir -p %s && mv %s %s/", destPath, filename, destPath)); err != nil {
		return fmt.Errorf("RunIssueCycle: move file: %w", err)
	}

	repoRelPath := strings.TrimPrefix(targetDir, "./") + "/" + filename

	if out, err := sb.Run(ctx, "cd /workspace && go build ./..."); err != nil {
		slog.Warn("go build failed", "error", err, "output", out)
	} else {
		slog.Info("build succeeded")
	}

	if out, err := sb.Run(ctx, "cd /workspace && go test ./..."); err != nil {
		slog.Warn("go test failed", "error", err, "output", out)
	} else {
		slog.Info("tests passed", "output", out)
	}

	commitMsg := fmt.Sprintf("feat: implement issue #%d", task.IssueNumber)
	commitCmd := fmt.Sprintf("cd /workspace && git add . && git commit -m %q", commitMsg)
	if out, err := sb.Run(ctx, commitCmd); err != nil {
		return fmt.Errorf("RunIssueCycle: git commit: %w", err)
	} else {
		slog.Info("committed", "output", out)
	}

	pushCmd := fmt.Sprintf("cd /workspace && git push origin %s", sess.BranchName)
	if out, err := sb.Run(ctx, pushCmd); err != nil {
		return fmt.Errorf("RunIssueCycle: git push: %w", err)
	} else {
		slog.Info("pushed branch", "branch", sess.BranchName, "output", out)
	}

	ghClient := newGitHubClient(ctx, cfg.GitHubToken)
	prTitle := fmt.Sprintf("feat: implement issue #%d", task.IssueNumber)
	prNum, err := CreatePR(ctx, ghClient, task.RepoOwner, task.RepoName, task.IssueNumber, prTitle, sess.BranchName, "main")
	if err != nil {
		return fmt.Errorf("RunIssueCycle: create PR: %w", err)
	}
	slog.Info("PR created", "pr_number", prNum)

	sess.Status = StatusReviewPending
	sess.PRNumber = prNum
	sess.GeneratedFile = repoRelPath
	if err := UpdateSession(ctx, redis, sess); err != nil {
		return fmt.Errorf("RunIssueCycle: update session: %w", err)
	}

	return nil
}

// RunFixCycle implements the fix workflow for a CodeRabbit review comment:
// read existing code → LLM fix → sandbox execution → push → session transition.
func RunFixCycle(ctx context.Context, cfg *Config, redis *RedisClient, task *Task, sess *AgentSession) error {
	slog.Info("running fix cycle", "issue_number", task.IssueNumber, "iteration", sess.Iteration)

	sb, err := NewSandbox(ctx, cfg.SandboxTemplate, cfg.SandboxNamespace)
	if err != nil {
		return fmt.Errorf("RunFixCycle: %w", err)
	}
	defer func() {
		if cerr := sb.Close(ctx); cerr != nil {
			slog.Error("sandbox close failed", "error", cerr)
		}
	}()

	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git",
		cfg.GitHubToken, task.RepoOwner, task.RepoName)

	cloneCmd := fmt.Sprintf("git clone -b %s %s /workspace", sess.BranchName, cloneURL)
	if out, err := sb.Run(ctx, cloneCmd); err != nil {
		return fmt.Errorf("RunFixCycle: git clone: %w", err)
	} else {
		slog.Info("repo cloned on branch", "branch", sess.BranchName, "output", out)
	}

	if _, err := sb.Run(ctx, "git config --global user.email 'agent@omakase.ai' && git config --global user.name 'Omakase Agent'"); err != nil {
		return fmt.Errorf("RunFixCycle: git config: %w", err)
	}

	var existingCode string
	if sess.GeneratedFile != "" {
		filePath := "/workspace/" + sess.GeneratedFile
		data, err := sb.Read(ctx, filePath)
		if err != nil {
			slog.Warn("could not read generated file, using empty context", "path", filePath, "error", err)
		} else {
			existingCode = string(data)
		}
	}

	if existingCode == "" {
		out, err := sb.Run(ctx, "find /workspace -name '*.go' -not -path '*/vendor/*' | head -5 | xargs cat 2>/dev/null || true")
		if err == nil {
			existingCode = out
		}
	}

	fixedCode, err := GenerateFixCode(ctx, cfg.AnthropicAPIKey, task.Body, existingCode)
	if err != nil {
		return fmt.Errorf("RunFixCycle: generate fix: %w", err)
	}
	slog.Info("fix code generated", "issue_number", task.IssueNumber)

	var fixFilename string
	if sess.GeneratedFile != "" {
		parts := strings.Split(sess.GeneratedFile, "/")
		fixFilename = parts[len(parts)-1]
	} else {
		fixFilename = fmt.Sprintf("fix_%d.go", task.IssueNumber)
	}

	if err := sb.Write(ctx, fixFilename, []byte(fixedCode)); err != nil {
		return fmt.Errorf("RunFixCycle: write fix: %w", err)
	}

	if sess.GeneratedFile != "" {
		destPath := "/workspace/" + sess.GeneratedFile
		mvCmd := fmt.Sprintf("mv %s %s", fixFilename, destPath)
		if _, err := sb.Run(ctx, mvCmd); err != nil {
			return fmt.Errorf("RunFixCycle: move fix file: %w", err)
		}
	}

	if out, err := sb.Run(ctx, "cd /workspace && go build ./..."); err != nil {
		slog.Warn("go build failed after fix", "error", err, "output", out)
	} else {
		slog.Info("build succeeded after fix")
	}

	if out, err := sb.Run(ctx, "cd /workspace && go test ./..."); err != nil {
		slog.Warn("go test failed after fix", "error", err, "output", out)
	} else {
		slog.Info("tests passed after fix", "output", out)
	}

	commitMsg := fmt.Sprintf("fix: apply review feedback (iteration %d)", sess.Iteration)
	commitCmd := fmt.Sprintf("cd /workspace && git add . && git commit -m %q", commitMsg)
	if out, err := sb.Run(ctx, commitCmd); err != nil {
		return fmt.Errorf("RunFixCycle: git commit: %w", err)
	} else {
		slog.Info("fix committed", "output", out)
	}

	pushCmd := fmt.Sprintf("cd /workspace && git push origin %s", sess.BranchName)
	if out, err := sb.Run(ctx, pushCmd); err != nil {
		return fmt.Errorf("RunFixCycle: git push: %w", err)
	} else {
		slog.Info("fix pushed", "branch", sess.BranchName, "output", out)
	}

	sess.Status = StatusDone
	if err := UpdateSession(ctx, redis, sess); err != nil {
		return fmt.Errorf("RunFixCycle: update session: %w", err)
	}

	slog.Info("fix cycle complete", "issue_number", task.IssueNumber, "status", sess.Status)
	return nil
}
