package issue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssueCreator_CreateArchivedActionIssue(t *testing.T) {
	var receivedRequests []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if !strings.Contains(r.URL.Path, "/repos/owner/repo/issues") {
			t.Errorf("Expected issues endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Expected Accept header, got %s", r.Header.Get("Accept"))
		}

		var issue map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&issue); err != nil {
			t.Errorf("Failed to decode issue: %v", err)
			return
		}

		receivedRequests = append(receivedRequests, issue)
		w.WriteHeader(201) // Created
	}))
	defer server.Close()

	creator := &IssueCreator{
		token:   "test-token",
		baseURL: server.URL,
	}
	// We need to set up the HTTP client to use our test server
	creator.SetHTTPClient(server.Client())

	actions := []ArchivedActionInfo{
		{
			Repo:     "actions/checkout",
			Workflow: "ci.yml",
			Uses:     "actions/checkout@v3",
		},
		{
			Repo:     "actions/setup-go",
			Workflow: "ci.yml",
			Uses:     "actions/setup-go@v4",
		},
	}

	ctx := context.Background()
	err := creator.CreateArchivedActionIssue(ctx, "owner", "repo", actions)

	if err != nil {
		t.Errorf("CreateArchivedActionIssue failed: %v", err)
	}

	if len(receivedRequests) != 1 {
		t.Errorf("Expected 1 issue creation request, got %d", len(receivedRequests))
	}

	issue := receivedRequests[0]
	if issue["title"] != "Replace archived GitHub Actions" {
		t.Errorf("Expected title 'Replace archived GitHub Actions', got '%s'", issue["title"])
	}

	body, ok := issue["body"].(string)
	if !ok {
		t.Error("Expected body to be string")
		return
	}

	if !strings.Contains(body, "actions/checkout@v3") {
		t.Error("Expected issue body to contain actions/checkout@v3")
	}

	if !strings.Contains(body, "actions/setup-go@v4") {
		t.Error("Expected issue body to contain actions/setup-go@v4")
	}

	labels, ok := issue["labels"].([]interface{})
	if !ok {
		t.Error("Expected labels to be array")
		return
	}

	expectedLabels := []string{"maintenance", "github-actions", "security"}
	if len(labels) != len(expectedLabels) {
		t.Errorf("Expected %d labels, got %d", len(expectedLabels), len(labels))
	}
}

func TestIssueCreator_buildIssueBody(t *testing.T) {
	creator := &IssueCreator{}

	actions := []ArchivedActionInfo{
		{
			Repo:     "actions/checkout",
			Workflow: "ci.yml",
			Uses:     "actions/checkout@v3",
		},
		{
			Repo:     "actions/setup-go",
			Workflow: "test.yml",
			Uses:     "actions/setup-go@v4",
		},
	}

	body := creator.buildIssueBody(actions)

	expectedContent := []string{
		"## Archived GitHub Actions Detected",
		"actions/checkout@v3",
		"actions/setup-go@v4",
		"## What does this mean?",
		"## Recommended Actions",
		"## Resources",
		"go-find-archived-gh-actions",
	}

	for _, content := range expectedContent {
		if !strings.Contains(body, content) {
			t.Errorf("Expected issue body to contain '%s'", content)
		}
	}
}

func TestNewIssueCreator(t *testing.T) {
	token := "test-token"
	creator := NewIssueCreator(token)

	if creator.token != token {
		t.Errorf("Expected token %s, got %s", token, creator.token)
	}

	if creator.baseURL != "https://api.github.com" {
		t.Errorf("Expected baseURL https://api.github.com, got %s", creator.baseURL)
	}

	if creator.client == nil {
		t.Error("Expected client to be set")
	}
}
