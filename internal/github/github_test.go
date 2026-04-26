package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_IsRepoArchived(t *testing.T) {
	tests := []struct {
		name         string
		ownerRepo    string
		responseBody string
		statusCode   int
		headers      map[string]string
		expected     bool
		expectError  bool
	}{
		{
			name:      "archived repository",
			ownerRepo: "owner/repo",
			responseBody: `{
				"name": "repo",
				"full_name": "owner/repo",
				"archived": true,
				"private": false,
				"html_url": "https://github.com/owner/repo"
			}`,
			statusCode:  200,
			expected:    true,
			expectError: false,
		},
		{
			name:      "active repository",
			ownerRepo: "owner/repo",
			responseBody: `{
				"name": "repo",
				"full_name": "owner/repo",
				"archived": false,
				"private": false,
				"html_url": "https://github.com/owner/repo"
			}`,
			statusCode:  200,
			expected:    false,
			expectError: false,
		},
		{
			name:        "repository not found",
			ownerRepo:   "owner/nonexistent",
			statusCode:  404,
			expected:    false,
			expectError: true,
		},
		{
			name:        "rate limited without reset time",
			ownerRepo:   "owner/repo",
			statusCode:  403,
			expected:    false,
			expectError: true,
		},
		{
			name:        "rate limited with reset time",
			ownerRepo:   "owner/repo",
			statusCode:  403,
			headers:     map[string]string{"X-RateLimit-Reset": "1640995200"},
			expected:    false,
			expectError: true,
		},
		{
			name:        "rate limited with bad reset time",
			ownerRepo:   "owner/repo",
			statusCode:  403,
			headers:     map[string]string{"X-RateLimit-Reset": "bad"},
			expected:    false,
			expectError: true,
		},
		{
			name:        "empty ownerRepo",
			ownerRepo:   "  ",
			expected:    false,
			expectError: true,
		},
		{
			name:        "invalid ownerRepo format",
			ownerRepo:   "owner",
			expected:    false,
			expectError: true,
		},
		{
			name:         "with https prefix and @ref",
			ownerRepo:    "https://github.com/owner/repo@v1",
			responseBody: `{"archived": true}`,
			statusCode:   200,
			expected:     true,
			expectError:  false,
		},
		{
			name:        "non 200/403/404 status",
			ownerRepo:   "owner/repo",
			statusCode:  500,
			expected:    false,
			expectError: true,
		},
		{
			name:         "bad json response",
			ownerRepo:    "owner/repo",
			responseBody: `invalid json`,
			statusCode:   200,
			expected:     false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}
				if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
					t.Errorf("Expected Accept header, got %s", r.Header.Get("Accept"))
				}
				if r.Header.Get("User-Agent") == "" {
					t.Errorf("Expected User-Agent header")
				}

				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}

				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
					if _, err := w.Write([]byte(tt.responseBody)); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
				}
			}))
			defer server.Close()

			client := &Client{
				httpClient: server.Client(),
				token:      "test-token",
				baseURL:    server.URL,
			}

			ctx := context.Background()
			archived, repoInfo, err := client.IsRepoArchived(ctx, tt.ownerRepo)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError {
				if archived != tt.expected {
					t.Errorf("Expected archived=%v, got %v", tt.expected, archived)
				}
				if tt.expected && repoInfo == nil {
					t.Error("Expected repo info for archived repo")
				}
			}
		})
	}
}

func TestClient_IsRepoArchived_RequestError(t *testing.T) {
	client := &Client{
		httpClient: http.DefaultClient,
		token:      "test",
		baseURL:    "http://127.0.0.1:0", // should fail to dial or connect
	}
	_, _, err := client.IsRepoArchived(context.Background(), "owner/repo")
	if err == nil {
		t.Error("Expected network error")
	}
}

func TestClient_IsRepoArchived_BadURL(t *testing.T) {
	client := &Client{
		httpClient: http.DefaultClient,
		token:      "test",
		baseURL:    "://bad\x00url",
	}
	_, _, err := client.IsRepoArchived(context.Background(), "owner/repo")
	if err == nil {
		t.Error("Expected URL creation error")
	}
}

func TestClient_CheckMultipleRepos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "error") {
			w.WriteHeader(500)
			return
		}
		response := `{"archived": false}`
		w.WriteHeader(200)
		// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
		if _, err := w.Write([]byte(response)); err != nil {
			t.Errorf("failed to write response body: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		token:      "test-token",
		baseURL:    server.URL,
	}

	ctx := context.Background()
	repos := []string{"owner/repo1", "owner/error_repo"}

	archived, errors := client.CheckMultipleRepos(ctx, repos)

	if len(archived) != 1 {
		t.Errorf("Expected 1 successful result, got %d", len(archived))
	}

	if len(errors) != 1 {
		t.Errorf("Expected 1 error result, got %d", len(errors))
	}
	if _, ok := errors["owner/error_repo"]; !ok {
		t.Error("Expected error for owner/error_repo")
	}
}

func TestNewClient(t *testing.T) {
	token := "test-token"
	client := NewClient(token)

	if client.token != token {
		t.Errorf("Expected token %s, got %s", token, client.token)
	}

	if client.baseURL != "https://api.github.com" {
		t.Errorf("Expected baseURL https://api.github.com, got %s", client.baseURL)
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be set")
	}
}

func TestClient_GetLatestRelease(t *testing.T) {
	tests := []struct {
		name         string
		ownerRepo    string
		responseBody string
		statusCode   int
		headers      map[string]string
		expectError  bool
		expectedTag  string
	}{
		{
			name:      "valid release",
			ownerRepo: "owner/repo",
			responseBody: `{
				"tag_name": "v1.2.3",
				"name": "Release 1.2.3",
				"draft": false,
				"prerelease": false,
				"html_url": "https://github.com/owner/repo/releases/tag/v1.2.3"
			}`,
			statusCode:  200,
			expectError: false,
			expectedTag: "v1.2.3",
		},
		{
			name:        "no releases found",
			ownerRepo:   "owner/repo",
			statusCode:  404,
			expectError: true,
		},
		{
			name:        "rate limited",
			ownerRepo:   "owner/repo",
			statusCode:  403,
			expectError: true,
		},
		{
			name:        "empty ownerRepo",
			ownerRepo:   "",
			expectError: true,
		},
		{
			name:        "invalid ownerRepo format",
			ownerRepo:   "invalid",
			expectError: true,
		},
		{
			name:         "with @ref suffix",
			ownerRepo:    "owner/repo@v1",
			responseBody: `{"tag_name": "v2.0.0"}`,
			statusCode:   200,
			expectError:  false,
			expectedTag:  "v2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}
				if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
					t.Errorf("Expected Accept header, got %s", r.Header.Get("Accept"))
				}

				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}

				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
					if _, err := w.Write([]byte(tt.responseBody)); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
				}
			}))
			defer server.Close()

			client := &Client{
				httpClient: server.Client(),
				token:      "test-token",
				baseURL:    server.URL,
			}

			ctx := context.Background()
			release, err := client.GetLatestRelease(ctx, tt.ownerRepo)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError && release != nil {
				if release.TagName != tt.expectedTag {
					t.Errorf("Expected tag %s, got %s", tt.expectedTag, release.TagName)
				}
			}
		})
	}
}

func TestClient_CheckMultipleReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "error") {
			w.WriteHeader(500)
			return
		}
		response := `{"tag_name": "v1.0.0"}`
		w.WriteHeader(200)
		// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
		if _, err := w.Write([]byte(response)); err != nil {
			t.Errorf("failed to write response body: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		token:      "test-token",
		baseURL:    server.URL,
	}

	ctx := context.Background()
	repos := []string{"owner/repo1", "owner/error_repo"}

	releases, errors := client.CheckMultipleReleases(ctx, repos)

	if len(releases) != 1 {
		t.Errorf("Expected 1 successful result, got %d", len(releases))
	}

	if len(errors) != 1 {
		t.Errorf("Expected 1 error result, got %d", len(errors))
	}
	if _, ok := errors["owner/error_repo"]; !ok {
		t.Error("Expected error for owner/error_repo")
	}
}

func TestClient_GetRefSHA(t *testing.T) {
	tests := []struct {
		name         string
		ownerRepo    string
		ref          string
		responseBody string
		statusCode   int
		wantSHA      string
		wantError    bool
	}{
		{
			name:      "tag exists",
			ownerRepo: "owner/repo",
			ref:       "v1.0.0",
			responseBody: `{
				"ref": "refs/tags/v1.0.0",
				"object": {
					"sha": "abc123def456",
					"type": "commit",
					"url": "https://api.github.com/repos/owner/repo/git/commits/abc123def456"
				}
			}`,
			statusCode: 200,
			wantSHA:    "abc123def456",
			wantError:  false,
		},
		{
			name:      "branch exists",
			ownerRepo: "owner/repo",
			ref:       "main",
			responseBody: `{
				"ref": "refs/heads/main",
				"object": {
					"sha": "def789ghi012",
					"type": "commit",
					"url": "https://api.github.com/repos/owner/repo/git/commits/def789ghi012"
				}
			}`,
			statusCode: 200,
			wantSHA:    "def789ghi012",
			wantError:  false,
		},
		{
			name:       "ref not found",
			ownerRepo:  "owner/repo",
			ref:        "nonexistent",
			statusCode: 404,
			wantError:  true,
		},
		{
			name:      "empty ownerRepo",
			ownerRepo: "",
			ref:       "v1",
			wantError: true,
		},
		{
			name:      "invalid ownerRepo format",
			ownerRepo: "invalid",
			ref:       "v1",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				// First try as tag, then as branch
				if callCount == 1 && tt.statusCode == 200 && strings.Contains(tt.name, "branch") {
					// First call (tag) returns 404 for branch test
					w.WriteHeader(404)
					return
				}
				if callCount == 2 && strings.Contains(tt.name, "tag") {
					// Second call (branch) for tag test should not happen
					t.Error("Should not try branch for tag test")
				}
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
					if _, err := w.Write([]byte(tt.responseBody)); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
				}
			}))
			defer server.Close()

			client := &Client{
				httpClient: server.Client(),
				token:      "test-token",
				baseURL:    server.URL,
			}

			ctx := context.Background()
			sha, err := client.GetRefSHA(ctx, tt.ownerRepo, tt.ref)

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.wantError && sha != tt.wantSHA {
				t.Errorf("Expected SHA %s, got %s", tt.wantSHA, sha)
			}
		})
	}
}

func TestClient_CompareRefSHAs(t *testing.T) {
	tests := []struct {
		name      string
		ownerRepo string
		ref1      string
		ref2      string
		responses []struct {
			path   string
			sha    string
			status int
		}
		wantSame  bool
		wantSHA1  string
		wantSHA2  string
		wantError bool
	}{
		{
			name:      "same SHA",
			ownerRepo: "owner/repo",
			ref1:      "v1",
			ref2:      "v1.0.0",
			responses: []struct {
				path   string
				sha    string
				status int
			}{
				{path: "v1", sha: "abc123", status: 200},
				{path: "v1.0.0", sha: "abc123", status: 200},
			},
			wantSame:  true,
			wantSHA1:  "abc123",
			wantSHA2:  "abc123",
			wantError: false,
		},
		{
			name:      "different SHA",
			ownerRepo: "owner/repo",
			ref1:      "v1",
			ref2:      "v1.0.1",
			responses: []struct {
				path   string
				sha    string
				status int
			}{
				{path: "v1", sha: "abc123", status: 200},
				{path: "v1.0.1", sha: "def456", status: 200},
			},
			wantSame:  false,
			wantSHA1:  "abc123",
			wantSHA2:  "def456",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if callCount < len(tt.responses) {
					resp := tt.responses[callCount]
					callCount++
					w.WriteHeader(resp.status)
					responseBody := fmt.Sprintf(`{
						"ref": "refs/tags/%s",
						"object": {
							"sha": "%s",
							"type": "commit"
						}
					}`, resp.path, resp.sha)
					// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
					if _, err := w.Write([]byte(responseBody)); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
				}
			}))
			defer server.Close()

			client := &Client{
				httpClient: server.Client(),
				token:      "test-token",
				baseURL:    server.URL,
			}

			ctx := context.Background()
			same, sha1, sha2, err := client.CompareRefSHAs(ctx, tt.ownerRepo, tt.ref1, tt.ref2)

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.wantError {
				if same != tt.wantSame {
					t.Errorf("Expected same=%v, got %v", tt.wantSame, same)
				}
				if sha1 != tt.wantSHA1 {
					t.Errorf("Expected SHA1 %s, got %s", tt.wantSHA1, sha1)
				}
				if sha2 != tt.wantSHA2 {
					t.Errorf("Expected SHA2 %s, got %s", tt.wantSHA2, sha2)
				}
			}
		})
	}
}
