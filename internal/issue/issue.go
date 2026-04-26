// Package issue provides functionality for creating GitHub issues
// when archived actions are detected in workflows.
//
// This package handles creating GitHub issues in repositories to notify
// maintainers about archived GitHub Actions that need replacement.
package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	log "github.com/sirupsen/logrus"
)

// IssueCreator handles creating GitHub issues.
type IssueCreator struct {
	token   string
	client  *http.Client
	baseURL string
}

// SetHTTPClient sets the HTTP client for testing purposes.
func (ic *IssueCreator) SetHTTPClient(client *http.Client) {
	ic.client = client
}

// NewIssueCreator creates a new IssueCreator with the provided GitHub token.
func NewIssueCreator(token string) *IssueCreator {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &IssueCreator{
		token:   token,
		client:  tc,
		baseURL: "https://api.github.com",
	}
}

// CreateArchivedActionIssue creates a GitHub issue about archived actions.
func (ic *IssueCreator) CreateArchivedActionIssue(ctx context.Context, owner, repo string, archivedActions []ArchivedActionInfo) error {
	if len(archivedActions) == 0 {
		return nil
	}

	title := "Replace archived GitHub Actions"
	body := ic.buildIssueBody(archivedActions)

	issue := map[string]interface{}{
		"title":  title,
		"body":   body,
		"labels": []string{"maintenance", "github-actions", "security"},
	}

	jsonData, err := json.Marshal(issue)
	if err != nil {
		return fmt.Errorf("failed to marshal issue data: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues", ic.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := ic.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 422 {
		log.Warnf("Issue may already exist in %s/%s", owner, repo)
		return nil // Don't treat as error if issue already exists
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub API returned status %d when creating issue", resp.StatusCode)
	}

	log.Infof("Successfully created GitHub issue in %s/%s", owner, repo)
	return nil
}

// ArchivedActionInfo represents information about an archived action.
type ArchivedActionInfo struct {
	Repo     string
	Workflow string
	Uses     string
}

// buildIssueBody creates the body content for the GitHub issue.
func (ic *IssueCreator) buildIssueBody(actions []ArchivedActionInfo) string {
	var body strings.Builder

	body.WriteString("## Archived GitHub Actions Detected\n\n")
	body.WriteString("This repository uses the following GitHub Actions that have been archived by their maintainers:\n\n")

	for _, action := range actions {
		body.WriteString(fmt.Sprintf("- `%s` (used in `%s`)\n", action.Uses, action.Workflow))
	}

	body.WriteString("\n## What does this mean?\n\n")
	body.WriteString("Archived actions are no longer maintained and may:\n")
	body.WriteString("- Contain security vulnerabilities\n")
	body.WriteString("- Stop working in future GitHub updates\n")
	body.WriteString("- Not receive bug fixes\n\n")

	body.WriteString("## Recommended Actions\n\n")
	body.WriteString("1. **Review each archived action** and find actively maintained alternatives\n")
	body.WriteString("2. **Test thoroughly** after replacing actions\n")
	body.WriteString("3. **Update your workflows** to use the new actions\n\n")

	body.WriteString("## Resources\n\n")
	body.WriteString("- [GitHub Actions Marketplace](https://github.com/marketplace?type=actions)\n")
	body.WriteString("- [Awesome Actions](https://github.com/sdras/awesome-actions)\n\n")

	body.WriteString("---\n\n")
	body.WriteString("*This issue was automatically created by [go-find-archived-gh-actions](https://github.com/toozej/go-find-archived-gh-actions)*\n")

	return body.String()
}
