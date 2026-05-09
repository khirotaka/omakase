package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// GenerateImplementationPlan calls the Anthropic API to produce an implementation plan from a GitHub issue body.
func GenerateImplementationPlan(ctx context.Context, apiKey, issueBody string) (string, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	prompt := fmt.Sprintf(`You are an expert software engineer. Based on the following GitHub issue, create a detailed implementation plan.

Issue:
%s

Provide a step-by-step implementation plan covering:
1. Files to create or modify
2. Functions/methods to implement
3. Dependencies and key considerations
4. Brief code outline where applicable`, issueBody)

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_7,
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("GenerateImplementationPlan: accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("GenerateImplementationPlan: %w", err)
	}

	plan := extractText(msg)
	if plan == "" {
		return "", fmt.Errorf("GenerateImplementationPlan: empty response from API")
	}
	return plan, nil
}

// GenerateFixPlan calls the Anthropic API to produce a fix plan from CodeRabbit review feedback and existing code.
func GenerateFixPlan(ctx context.Context, apiKey, reviewFeedback, existingCode string) (string, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	prompt := fmt.Sprintf(`You are an expert software engineer. Based on the following code review feedback and existing code, create a detailed fix plan.

Review Feedback:
%s

Existing Code:
%s

Provide a step-by-step fix plan covering:
1. Specific changes required
2. Which files, functions, or lines to modify
3. Corrected code snippets where applicable
4. Any additional improvements the review suggests`, reviewFeedback, existingCode)

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_7,
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("GenerateFixPlan: accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("GenerateFixPlan: %w", err)
	}

	plan := extractText(msg)
	if plan == "" {
		return "", fmt.Errorf("GenerateFixPlan: empty response from API")
	}
	return plan, nil
}

// GenerateCode calls the Anthropic API to produce a Go source file from an issue body and plan.
// Returns the filename, target directory (relative to /workspace), and the source code.
func GenerateCode(ctx context.Context, apiKey, issueBody, plan string) (filename, targetDir, code string, err error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	prompt := fmt.Sprintf(`You are an expert Go software engineer. Based on the following GitHub issue and implementation plan, write the Go source code.

Issue:
%s

Implementation Plan:
%s

Respond in EXACTLY this format (no extra text before or after):
FILENAME: <filename.go>
TARGET_DIR: <relative/dir/>

`+"```go"+`
<complete go source code>
`+"```"+`

Rules:
- FILENAME must be a single .go file name with no path separators
- TARGET_DIR is the relative directory within the repository where this file belongs (e.g., "agent/" or "./"); end with /
- The code block must contain valid, compilable Go source code`, issueBody, plan)

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_7,
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", "", "", fmt.Errorf("GenerateCode: accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", "", "", fmt.Errorf("GenerateCode: %w", err)
	}

	response := extractText(msg)
	if response == "" {
		return "", "", "", fmt.Errorf("GenerateCode: empty response from API")
	}

	filename, targetDir, code, err = parseCodeResponse(response)
	if err != nil {
		return "", "", "", fmt.Errorf("GenerateCode: %w", err)
	}
	return filename, targetDir, code, nil
}

// GenerateFixCode calls the Anthropic API to produce a corrected Go source file
// based on code review feedback and the existing code.
func GenerateFixCode(ctx context.Context, apiKey, reviewFeedback, existingCode string) (string, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	prompt := fmt.Sprintf(`You are an expert Go software engineer. Fix the following Go code based on the code review feedback.

Code Review Feedback:
%s

Existing Code:
%s

Respond with ONLY the corrected Go source code in a code block:
`+"```go"+`
<corrected go source code>
`+"```", reviewFeedback, existingCode)

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_7,
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("GenerateFixCode: accumulate: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("GenerateFixCode: %w", err)
	}

	response := extractText(msg)
	if response == "" {
		return "", fmt.Errorf("GenerateFixCode: empty response from API")
	}

	code := extractCodeBlock(response)
	if code == "" {
		return "", fmt.Errorf("GenerateFixCode: no code block in response")
	}
	return code, nil
}

// parseCodeResponse parses a structured LLM response containing FILENAME, TARGET_DIR, and a Go code block.
func parseCodeResponse(response string) (filename, targetDir, code string, err error) {
	lines := strings.Split(response, "\n")
	inCode := false
	var codeLines []string

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "FILENAME: "):
			filename = strings.TrimSpace(strings.TrimPrefix(line, "FILENAME: "))
		case strings.HasPrefix(line, "TARGET_DIR: "):
			targetDir = strings.TrimSpace(strings.TrimPrefix(line, "TARGET_DIR: "))
		case line == "```go" && !inCode:
			inCode = true
		case line == "```" && inCode:
			code = strings.Join(codeLines, "\n")
			inCode = false
		case inCode:
			codeLines = append(codeLines, line)
		}
	}

	if filename == "" {
		return "", "", "", fmt.Errorf("parseCodeResponse: FILENAME not found in response")
	}
	if code == "" {
		return "", "", "", fmt.Errorf("parseCodeResponse: no Go code block found in response")
	}
	if targetDir == "" {
		targetDir = "./"
	}
	return filename, targetDir, code, nil
}

// extractCodeBlock extracts the first Go code block from a response string.
func extractCodeBlock(response string) string {
	lines := strings.Split(response, "\n")
	inCode := false
	var codeLines []string

	for _, line := range lines {
		if line == "```go" && !inCode {
			inCode = true
			continue
		}
		if line == "```" && inCode {
			break
		}
		if inCode {
			codeLines = append(codeLines, line)
		}
	}
	return strings.Join(codeLines, "\n")
}

func extractText(msg anthropic.Message) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}
