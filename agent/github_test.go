package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v67/github"
)

func newTestGitHubClient(t *testing.T, srv *httptest.Server) *github.Client {
	t.Helper()
	client := github.NewClient(srv.Client())
	baseURL, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	client.BaseURL = baseURL
	return client
}

func TestCreatePR(t *testing.T) {
	var gotReqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/repos/owner/repo/pulls") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReqBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"number": 99, "html_url": "https://github.com/owner/repo/pull/99"}`)
	}))
	defer srv.Close()

	client := newTestGitHubClient(t, srv)
	prNumber, err := CreatePR(context.Background(), client, "owner", "repo", 42, "Fix issue", "agent/issue-42", "main")
	if err != nil {
		t.Fatalf("CreatePR error: %v", err)
	}
	if prNumber != 99 {
		t.Errorf("prNumber = %d, want 99", prNumber)
	}

	body, ok := gotReqBody["body"].(string)
	if !ok {
		t.Fatal("request body missing 'body' field")
	}
	if !strings.Contains(body, "Closes #42") {
		t.Errorf("PR body %q does not contain 'Closes #42'", body)
	}
	if gotReqBody["head"] != "agent/issue-42" {
		t.Errorf("head = %v, want agent/issue-42", gotReqBody["head"])
	}
	if gotReqBody["base"] != "main" {
		t.Errorf("base = %v, want main", gotReqBody["base"])
	}
}

func TestCreatePR_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Unprocessable Entity"}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	client := newTestGitHubClient(t, srv)
	_, err := CreatePR(context.Background(), client, "owner", "repo", 1, "title", "branch", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPostIssueComment(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/repos/owner/repo/issues/7/comments") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id": 1, "body": "hello"}`)
	}))
	defer srv.Close()

	client := newTestGitHubClient(t, srv)
	err := PostIssueComment(context.Background(), client, "owner", "repo", 7, "hello")
	if err != nil {
		t.Fatalf("PostIssueComment error: %v", err)
	}
	if gotBody["body"] != "hello" {
		t.Errorf("comment body = %v, want hello", gotBody["body"])
	}
}

func TestPostIssueComment_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestGitHubClient(t, srv)
	err := PostIssueComment(context.Background(), client, "owner", "repo", 999, "msg")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
