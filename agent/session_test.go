package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRedisStore is a minimal in-memory store for testing session operations
// without a real Redis. It intercepts the Upstash REST API calls.
func newFakeRedisServer(t *testing.T) (*httptest.Server, map[string]string) {
	t.Helper()
	store := map[string]string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// GET /get/<key>
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/get/") {
			key := strings.TrimPrefix(r.URL.Path, "/get/")
			val, ok := store[key]
			if !ok {
				_, _ = fmt.Fprint(w, `{"result":null}`)
				return
			}
			resp, _ := json.Marshal(map[string]any{"result": val})
			_, _ = w.Write(resp)
			return
		}

		// POST / — command-in-body (used by SET)
		if r.Method == http.MethodPost && r.URL.Path == "/" {
			var cmd []json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			var op string
			_ = json.Unmarshal(cmd[0], &op)
			op = strings.ToUpper(op)

			if op == "SET" && len(cmd) >= 3 {
				var key, value string
				_ = json.Unmarshal(cmd[1], &key)
				_ = json.Unmarshal(cmd[2], &value)
				store[key] = value
				_, _ = fmt.Fprint(w, `{"result":"OK"}`)
				return
			}
			http.Error(w, "unsupported command", http.StatusBadRequest)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	return srv, store
}

func TestCreateSession(t *testing.T) {
	srv, store := newFakeRedisServer(t)
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	sess, err := CreateSession(context.Background(), client, 42)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if sess.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", sess.IssueNumber)
	}
	if sess.Status != StatusDeveloping {
		t.Errorf("Status = %q, want %q", sess.Status, StatusDeveloping)
	}
	if sess.Iteration != 0 {
		t.Errorf("Iteration = %d, want 0", sess.Iteration)
	}
	if sess.BranchName != "agent/issue-42" {
		t.Errorf("BranchName = %q, want %q", sess.BranchName, "agent/issue-42")
	}

	// Verify the value is persisted in the store
	if _, ok := store["session:42"]; !ok {
		t.Error("session:42 not found in Redis store")
	}
}

func TestReadSession(t *testing.T) {
	srv, store := newFakeRedisServer(t)
	defer srv.Close()

	stored := AgentSession{
		IssueNumber: 7,
		Status:      StatusDeveloping,
		BranchName:  "agent/issue-7",
		Iteration:   0,
	}
	data, _ := json.Marshal(stored)
	store["session:7"] = string(data)

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	sess, err := ReadSession(context.Background(), client, 7)
	if err != nil {
		t.Fatalf("ReadSession error: %v", err)
	}
	if sess == nil {
		t.Fatal("ReadSession returned nil, want a session")
	}
	if sess.Status != StatusDeveloping {
		t.Errorf("Status = %q, want %q", sess.Status, StatusDeveloping)
	}
	if sess.BranchName != "agent/issue-7" {
		t.Errorf("BranchName = %q, want %q", sess.BranchName, "agent/issue-7")
	}
}

func TestReadSession_NotFound(t *testing.T) {
	srv, _ := newFakeRedisServer(t)
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	sess, err := ReadSession(context.Background(), client, 999)
	if err != nil {
		t.Fatalf("ReadSession error: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil session for missing key, got %+v", sess)
	}
}

func TestUpdateSession(t *testing.T) {
	srv, store := newFakeRedisServer(t)
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())

	sess := &AgentSession{
		IssueNumber: 10,
		Status:      StatusDeveloping,
		BranchName:  "agent/issue-10",
		Iteration:   0,
	}
	data, _ := json.Marshal(sess)
	store["session:10"] = string(data)

	sess.Status = StatusReviewPending
	sess.PRNumber = 55
	if err := UpdateSession(context.Background(), client, sess); err != nil {
		t.Fatalf("UpdateSession error: %v", err)
	}

	// Verify persisted value
	var updated AgentSession
	_ = json.Unmarshal([]byte(store["session:10"]), &updated)
	if updated.Status != StatusReviewPending {
		t.Errorf("Status = %q, want %q", updated.Status, StatusReviewPending)
	}
	if updated.PRNumber != 55 {
		t.Errorf("PRNumber = %d, want 55", updated.PRNumber)
	}
}

func TestRedisClient_SET_GET(t *testing.T) {
	srv, _ := newFakeRedisServer(t)
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())

	if err := client.SET(context.Background(), "mykey", `{"hello":"world"}`, 0); err != nil {
		t.Fatalf("SET error: %v", err)
	}

	val, err := client.GET(context.Background(), "mykey")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	if val != `{"hello":"world"}` {
		t.Errorf("GET = %q, want %q", val, `{"hello":"world"}`)
	}
}

func TestRedisClient_GET_Missing(t *testing.T) {
	srv, _ := newFakeRedisServer(t)
	defer srv.Close()

	client := NewRedisClient(srv.URL, "test-token", srv.Client())
	val, err := client.GET(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	if val != "" {
		t.Errorf("GET = %q, want empty string for missing key", val)
	}
}
