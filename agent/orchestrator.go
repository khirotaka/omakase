package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// validateRelPath rejects paths that could escape /workspace or inject shell commands.
func validateRelPath(p string) error {
	cleaned := filepath.Clean(p)
	if filepath.IsAbs(cleaned) || strings.Contains(cleaned, "..") {
		return fmt.Errorf("unsafe path %q: must be a relative path without traversal", p)
	}
	if strings.ContainsAny(p, ";&|$><\\`'\"") {
		return fmt.Errorf("unsafe path %q: contains shell metacharacters", p)
	}
	return nil
}

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

	if err := validateRelPath(targetDir); err != nil {
		return fmt.Errorf("RunIssueCycle: invalid target dir: %w", err)
	}
	if err := validateRelPath(filename); err != nil {
		return fmt.Errorf("RunIssueCycle: invalid filename: %w", err)
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

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", task.RepoOwner, task.RepoName)
	cloneCmd := fmt.Sprintf("git -c http.extraHeader='Authorization: bearer %s' clone %s /workspace",
		cfg.GitHubToken, repoURL)
	if _, err := sb.RunSensitive(ctx, cloneCmd, fmt.Sprintf("git clone %s /workspace", repoURL)); err != nil {
		return fmt.Errorf("RunIssueCycle: git clone: %w", err)
	}
	slog.Info("repo cloned", "repo", repoURL)

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

	cleanTarget := filepath.Clean(targetDir)
	destPath := filepath.Join("/workspace", cleanTarget)
	repoRelPath := filepath.Join(cleanTarget, filename)

	mvCmd := fmt.Sprintf("mkdir -p '%s' && mv '%s' '%s/'", destPath, filename, destPath)
	if _, err := sb.Run(ctx, mvCmd); err != nil {
		return fmt.Errorf("RunIssueCycle: move file: %w", err)
	}

	if out, err := sb.Run(ctx, "cd /workspace && go build ./..."); err != nil {
		return fmt.Errorf("RunIssueCycle: go build failed: %w\n%s", err, out)
	}
	slog.Info("build succeeded")

	if out, err := sb.Run(ctx, "cd /workspace && go test ./..."); err != nil {
		return fmt.Errorf("RunIssueCycle: go test failed: %w\n%s", err, out)
	}
	slog.Info("tests passed")

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

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", task.RepoOwner, task.RepoName)
	cloneCmd := fmt.Sprintf("git -c http.extraHeader='Authorization: bearer %s' clone -b %s %s /workspace",
		cfg.GitHubToken, sess.BranchName, repoURL)
	displayClone := fmt.Sprintf("git clone -b %s %s /workspace", sess.BranchName, repoURL)
	if _, err := sb.RunSensitive(ctx, cloneCmd, displayClone); err != nil {
		return fmt.Errorf("RunFixCycle: git clone: %w", err)
	}
	slog.Info("repo cloned on branch", "branch", sess.BranchName, "repo", repoURL)

	if _, err := sb.Run(ctx, "git config --global user.email 'agent@omakase.ai' && git config --global user.name 'Omakase Agent'"); err != nil {
		return fmt.Errorf("RunFixCycle: git config: %w", err)
	}

	var existingCode string
	if sess.GeneratedFile != "" {
		if err := validateRelPath(sess.GeneratedFile); err != nil {
			return fmt.Errorf("RunFixCycle: invalid generated file path: %w", err)
		}
		filePath := filepath.Join("/workspace", sess.GeneratedFile)
		data, err := sb.Read(ctx, filePath)
		if err != nil {
			slog.Warn("could not read generated file, using empty context", "error", err)
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
		fixFilename = filepath.Base(sess.GeneratedFile)
	} else {
		fixFilename = fmt.Sprintf("fix_%d.go", task.IssueNumber)
	}

	if err := sb.Write(ctx, fixFilename, []byte(fixedCode)); err != nil {
		return fmt.Errorf("RunFixCycle: write fix: %w", err)
	}

	if sess.GeneratedFile != "" {
		destPath := filepath.Join("/workspace", sess.GeneratedFile)
		mvCmd := fmt.Sprintf("mv '%s' '%s'", fixFilename, destPath)
		if _, err := sb.Run(ctx, mvCmd); err != nil {
			return fmt.Errorf("RunFixCycle: move fix file: %w", err)
		}
	}

	if out, err := sb.Run(ctx, "cd /workspace && go build ./..."); err != nil {
		return fmt.Errorf("RunFixCycle: go build failed: %w\n%s", err, out)
	}
	slog.Info("build succeeded after fix")

	if out, err := sb.Run(ctx, "cd /workspace && go test ./..."); err != nil {
		return fmt.Errorf("RunFixCycle: go test failed: %w\n%s", err, out)
	}
	slog.Info("tests passed after fix")

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
