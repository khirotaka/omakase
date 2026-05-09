package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedisClient_RPOP_WithItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/rpop/agent-queue" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{"result":"{\"type\":\"issue\",\"issueNumber\":42,\"repoOwner\":\"khirotaka\",\"repoName\":\"omakase\",\"body\":\"hello\"}"}`)
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	val, err := client.RPOP(context.Background(), "agent-queue")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == "" {
		t.Fatal("expected non-empty result")
	}

	task, err := parseTask(val)
	if err != nil {
		t.Fatalf("parseTask error: %v", err)
	}
	if task.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", task.IssueNumber)
	}
	if task.Type != TaskTypeIssue {
		t.Errorf("Type = %q, want %q", task.Type, TaskTypeIssue)
	}
}

func TestRedisClient_RPOP_EmptyQueue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{"result":null}`)
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	val, err := client.RPOP(context.Background(), "agent-queue")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for null result, got %q", val)
	}
}

func TestRedisClient_RPOP_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewRedisClient(srv.URL, "bad-token", srv.Client())
	_, err := client.RPOP(context.Background(), "agent-queue")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestRedisClient_RPOP_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// deliberately never responds
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	_, err := client.RPOP(ctx, "agent-queue")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
