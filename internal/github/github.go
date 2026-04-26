// Package github provides functionality for interacting with the GitHub API
// to check if repositories are archived.
//
// This package handles GitHub API authentication, repository information retrieval,
// and archived status checking for GitHub Actions used in workflows.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Client represents a GitHub API client.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

// RepoInfo represents the information returned by the GitHub API for a repository.
type RepoInfo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Archived bool   `json:"archived"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
	Owner    Owner  `json:"owner"`
}

// ReleaseInfo represents a GitHub release.
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// Owner represents the owner of a GitHub repository.
type Owner struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// NewClient creates a new GitHub API client with the provided token.
func NewClient(token string) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		httpClient: tc,
		token:      token,
		baseURL:    "https://api.github.com",
	}
}

// IsRepoArchived checks if a GitHub repository is archived.
// It takes an owner/repo string and returns whether it's archived, the full repo info, and any error.
func (c *Client) IsRepoArchived(ctx context.Context, ownerRepo string) (bool, *RepoInfo, error) {
	// Clean the owner/repo string
	ownerRepo = strings.TrimSpace(ownerRepo)
	if ownerRepo == "" {
		return false, nil, fmt.Errorf("empty owner/repo string")
	}

	// Remove any leading "https://github.com/"
	ownerRepo = strings.TrimPrefix(ownerRepo, "https://github.com/")

	// Remove any @ref suffix to get clean owner/repo
	if idx := strings.Index(ownerRepo, "@"); idx != -1 {
		ownerRepo = ownerRepo[:idx]
	}

	// Split into owner and repo
	parts := strings.Split(ownerRepo, "/")
	if len(parts) != 2 {
		return false, nil, fmt.Errorf("invalid owner/repo format: %s", ownerRepo)
	}
	owner, repo := parts[0], parts[1]

	// Make API request
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "go-find-archived-gh-actions")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limiting
	if resp.StatusCode == 403 {
		resetTime := resp.Header.Get("X-RateLimit-Reset")
		if resetTime != "" {
			if resetUnix, err := strconv.ParseInt(resetTime, 10, 64); err == nil {
				reset := time.Unix(resetUnix, 0)
				return false, nil, fmt.Errorf("rate limited, resets at %s", reset.Format(time.RFC3339))
			}
		}
		return false, nil, fmt.Errorf("rate limited by GitHub API")
	}

	if resp.StatusCode == 404 {
		return false, nil, fmt.Errorf("repository %s/%s not found", owner, repo)
	}

	if resp.StatusCode != 200 {
		return false, nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Parse response
	var repoInfo RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return false, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return repoInfo.Archived, &repoInfo, nil
}

// CheckMultipleRepos checks multiple repositories for archived status.
// It returns a map of owner/repo to archived status and any errors encountered.
func (c *Client) CheckMultipleRepos(ctx context.Context, repos []string) (map[string]bool, map[string]error) {
	archived := make(map[string]bool)
	errors := make(map[string]error)

	for _, repo := range repos {
		isArchived, _, err := c.IsRepoArchived(ctx, repo)
		if err != nil {
			errors[repo] = err
			continue
		}
		archived[repo] = isArchived

		// Add small delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return archived, errors
}

// GetLatestRelease fetches the latest release for a repository.
// It returns the release info or an error if the repository has no releases.
func (c *Client) GetLatestRelease(ctx context.Context, ownerRepo string) (*ReleaseInfo, error) {
	// Clean the owner/repo string
	ownerRepo = strings.TrimSpace(ownerRepo)
	if ownerRepo == "" {
		return nil, fmt.Errorf("empty owner/repo string")
	}

	// Remove any @ref suffix to get clean owner/repo
	if idx := strings.Index(ownerRepo, "@"); idx != -1 {
		ownerRepo = ownerRepo[:idx]
	}

	// Split into owner and repo
	parts := strings.Split(ownerRepo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid owner/repo format: %s", ownerRepo)
	}
	owner, repo := parts[0], parts[1]

	// Make API request
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "go-find-archived-gh-actions")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limiting
	if resp.StatusCode == 403 {
		resetTime := resp.Header.Get("X-RateLimit-Reset")
		if resetTime != "" {
			if resetUnix, err := strconv.ParseInt(resetTime, 10, 64); err == nil {
				reset := time.Unix(resetUnix, 0)
				return nil, fmt.Errorf("rate limited, resets at %s", reset.Format(time.RFC3339))
			}
		}
		return nil, fmt.Errorf("rate limited by GitHub API")
	}

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no releases found for %s/%s", owner, repo)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Parse response
	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &release, nil
}

// CheckMultipleReleases fetches the latest releases for multiple repositories.
// It returns a map of owner/repo to release info and any errors encountered.
func (c *Client) CheckMultipleReleases(ctx context.Context, repos []string) (map[string]*ReleaseInfo, map[string]error) {
	releases := make(map[string]*ReleaseInfo)
	errors := make(map[string]error)

	for _, repo := range repos {
		release, err := c.GetLatestRelease(ctx, repo)
		if err != nil {
			errors[repo] = err
			continue
		}
		releases[repo] = release

		// Add small delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return releases, errors
}

// TagInfo represents information about a Git tag.
type TagInfo struct {
	Name      string `json:"name"`
	CommitSHA string `json:"sha"`
	CommitURL string `json:"url"`
}

// RefInfo represents information about a Git ref (branch, tag, or commit).
type RefInfo struct {
	Ref    string `json:"ref"`
	Object struct {
		SHA  string `json:"sha"`
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"object"`
}

// GetRefSHA fetches the commit SHA for a given ref (tag, branch, or commit) in a repository.
func (c *Client) GetRefSHA(ctx context.Context, ownerRepo, ref string) (string, error) {
	ownerRepo = strings.TrimSpace(ownerRepo)
	if ownerRepo == "" {
		return "", fmt.Errorf("empty owner/repo string")
	}

	// Remove any @ref suffix from ownerRepo
	if idx := strings.Index(ownerRepo, "@"); idx != -1 {
		ownerRepo = ownerRepo[:idx]
	}

	parts := strings.Split(ownerRepo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid owner/repo format: %s", ownerRepo)
	}
	owner, repo := parts[0], parts[1]

	// Try as a tag first (refs/tags/v1)
	url := fmt.Sprintf("%s/repos/%s/%s/git/refs/tags/%s", c.baseURL, owner, repo, ref)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "go-find-archived-gh-actions")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var refInfo RefInfo
		if err := json.NewDecoder(resp.Body).Decode(&refInfo); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}
		return refInfo.Object.SHA, nil
	}

	// Try as a branch (refs/heads/main)
	url = fmt.Sprintf("%s/repos/%s/%s/git/refs/heads/%s", c.baseURL, owner, repo, ref)
	req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "go-find-archived-gh-actions")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var refInfo RefInfo
		if err := json.NewDecoder(resp.Body).Decode(&refInfo); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}
		return refInfo.Object.SHA, nil
	}

	// Handle rate limiting
	if resp.StatusCode == 403 {
		resetTime := resp.Header.Get("X-RateLimit-Reset")
		if resetTime != "" {
			if resetUnix, err := strconv.ParseInt(resetTime, 10, 64); err == nil {
				reset := time.Unix(resetUnix, 0)
				return "", fmt.Errorf("rate limited, resets at %s", reset.Format(time.RFC3339))
			}
		}
		return "", fmt.Errorf("rate limited by GitHub API")
	}

	return "", fmt.Errorf("ref %s not found in %s/%s", ref, owner, repo)
}

// CompareRefSHAs compares the commit SHAs of two refs in the same repository.
// Returns true if the SHAs are identical, false if different.
func (c *Client) CompareRefSHAs(ctx context.Context, ownerRepo, ref1, ref2 string) (bool, string, string, error) {
	sha1, err := c.GetRefSHA(ctx, ownerRepo, ref1)
	if err != nil {
		return false, "", "", fmt.Errorf("failed to get SHA for ref %s: %w", ref1, err)
	}

	// Add small delay to avoid rate limiting
	time.Sleep(100 * time.Millisecond)

	sha2, err := c.GetRefSHA(ctx, ownerRepo, ref2)
	if err != nil {
		return false, sha1, "", fmt.Errorf("failed to get SHA for ref %s: %w", ref2, err)
	}

	return sha1 == sha2, sha1, sha2, nil
}
