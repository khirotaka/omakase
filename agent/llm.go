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

func extractText(msg anthropic.Message) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}
