package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"sigs.k8s.io/agent-sandbox/clients/go/sandbox"
)

// Sandbox wraps the agent-sandbox client and enforces project conventions.
type Sandbox struct {
	sb *sandbox.Sandbox
}

// NewSandbox creates and opens a new sandbox Pod from the given SandboxTemplate.
func NewSandbox(ctx context.Context, templateName, namespace string) (*Sandbox, error) {
	opts := sandbox.Options{
		TemplateName: templateName,
		Namespace:    namespace,
	}

	sb, err := sandbox.New(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("NewSandbox: %w", err)
	}

	if err := sb.Open(ctx); err != nil {
		return nil, fmt.Errorf("NewSandbox open: %w", err)
	}

	slog.Info("sandbox started", "claim", sb.ClaimName(), "sandbox", sb.SandboxName())
	return &Sandbox{sb: sb}, nil
}

// Run executes a shell command in the sandbox and returns the combined output.
// Returns an error if the command exits with a non-zero code.
func (s *Sandbox) Run(ctx context.Context, cmd string) (string, error) {
	result, err := s.sb.Run(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("sandbox run %q: %w", cmd, err)
	}

	output := strings.TrimSpace(result.Stdout + result.Stderr)
	if result.ExitCode != 0 {
		return output, fmt.Errorf("sandbox run %q: exit code %d: %s", cmd, result.ExitCode, output)
	}

	return output, nil
}

// Write uploads content to the sandbox root directory using a filename only.
// Path separators are not allowed; use Run("mv ...") to relocate the file.
func (s *Sandbox) Write(ctx context.Context, filename string, content []byte) error {
	if strings.ContainsRune(filename, '/') {
		return fmt.Errorf("sandbox write: filename %q must not contain path separators; use Run to move the file", filename)
	}

	if err := s.sb.Write(ctx, filename, content); err != nil {
		return fmt.Errorf("sandbox write %q: %w", filename, err)
	}

	return nil
}

// Read downloads the contents of a file at the given sandbox path.
func (s *Sandbox) Read(ctx context.Context, path string) ([]byte, error) {
	data, err := s.sb.Read(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("sandbox read %q: %w", path, err)
	}

	return data, nil
}

// Close terminates the sandbox Pod and deletes the SandboxClaim.
func (s *Sandbox) Close(ctx context.Context) error {
	if err := s.sb.Close(ctx); err != nil {
		return fmt.Errorf("sandbox close: %w", err)
	}

	slog.Info("sandbox closed", "claim", s.sb.ClaimName())
	return nil
}
