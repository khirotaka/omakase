package main

import (
	"encoding/json"
	"fmt"
)

// TaskType represents the kind of work to perform.
type TaskType string

const (
	TaskTypeIssue  TaskType = "issue"
	TaskTypeReview TaskType = "review"
)

// Task is the payload pushed to agent-queue via LPUSH and dequeued via RPOP.
type Task struct {
	Type        TaskType `json:"type"`
	IssueNumber int      `json:"issueNumber"`
	RepoOwner   string   `json:"repoOwner"`
	RepoName    string   `json:"repoName"`
	Body        string   `json:"body"`
}

func parseTask(raw string) (*Task, error) {
	var t Task
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, fmt.Errorf("parseTask: %w", err)
	}
	return &t, nil
}
