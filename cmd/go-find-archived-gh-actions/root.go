// Package cmd provides command-line interface functionality for the go-find-archived-gh-actions application.
//
// This package implements the root command and manages the command-line interface
// using the cobra library. It handles configuration, logging setup, and command
// execution for detecting archived GitHub Actions in workflows.
//
// The package integrates with several components:
//   - Configuration management through pkg/config
//   - Workflow parsing through internal/workflow
//   - GitHub API client through internal/github
//   - Notification system through internal/notification
//   - Issue creation through internal/issue
//
// Example usage:
//
//	import "github.com/toozej/go-find-archived-gh-actions/cmd/go-find-archived-gh-actions"
//
//	func main() {
//		cmd.Execute()
//	}
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/toozej/go-find-archived-gh-actions/internal/github"
	"github.com/toozej/go-find-archived-gh-actions/internal/issue"
	"github.com/toozej/go-find-archived-gh-actions/internal/notification"
	ver "github.com/toozej/go-find-archived-gh-actions/internal/version"
	"github.com/toozej/go-find-archived-gh-actions/internal/workflow"
	"github.com/toozej/go-find-archived-gh-actions/pkg/config"
	"github.com/toozej/go-find-archived-gh-actions/pkg/man"
	"github.com/toozej/go-find-archived-gh-actions/pkg/version"
)

// conf holds the application configuration loaded from environment variables.
// It is populated during package initialization and can be modified by command-line flags.
var (
	conf          config.Config
	debug         bool
	verbose       bool
	workflowPath  string
	githubToken   string
	notify        bool
	createIssue   bool
	checkOutdated bool
)

// rootCmd defines the base command for the go-find-archived-gh-actions CLI application.
// It detects archived GitHub Actions in repository workflows.
var rootCmd = &cobra.Command{
	Use:   "go-find-archived-gh-actions",
	Short: "Detect archived GitHub Actions in repository workflows",
	Long: `A tool to detect if GitHub Actions used in repository workflows have been archived upstream.

The tool scans .github/workflows/**/*.yml and **/*.yaml files, extracts 'uses:' references,
checks the GitHub API for archived status, and reports findings.

Exit codes:
  0 - No archived actions found
  1 - Archived actions found or error occurred`,
	Args:             cobra.NoArgs,
	PersistentPreRun: rootCmdPreRun,
	Run:              rootCmdRun,
}

// rootCmdRun is the main execution function for the root command.
// It implements the core logic for detecting archived GitHub Actions.
func rootCmdRun(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Determine working directory
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Get GitHub token
	token := conf.GitHubToken
	if token == "" {
		token = conf.GitHubTokenFallback
	}
	if githubToken != "" {
		token = githubToken
	}
	if token == "" {
		log.Fatal("GitHub token not provided. Set GH_TOKEN or GITHUB_TOKEN environment variable, or use --token flag")
	}

	// Initialize components
	parser := workflow.NewParser()
	ghClient := github.NewClient(token)
	var notifier *notification.NotificationManager
	var issueCreator *issue.IssueCreator

	if notify {
		manager, err := notification.NewNotificationManager(conf.Notification)
		if err != nil {
			log.Fatalf("Failed to initialize notification manager: %v", err)
		}
		notifier = manager
	}

	if createIssue {
		issueCreator = issue.NewIssueCreator(token)
	}

	// Find workflows
	var workflowFiles []*workflow.WorkflowFile
	var allActionRefs []workflow.ActionRef

	if workflowPath != "" {
		// Check specific workflow file
		if !filepath.IsAbs(workflowPath) {
			workflowPath = filepath.Join(workDir, workflowPath)
		}

		workflowFile, err := parser.ParseWorkflowFile(workflowPath)
		if err != nil {
			log.Fatalf("Failed to parse workflow file %s: %v", workflowPath, err)
		}
		workflowFiles = append(workflowFiles, workflowFile)
		allActionRefs = append(allActionRefs, workflowFile.UsesWithVersions...)
	} else {
		// Find all workflow files
		actionRefs, workflows, err := parser.GetAllUsesFromRepoWithVersions(workDir)
		if err != nil {
			log.Fatalf("Failed to find workflow files: %v", err)
		}
		workflowFiles = workflows
		allActionRefs = actionRefs
	}

	if verbose {
		fmt.Printf("Found %d workflow files\n", len(workflowFiles))
		for _, wf := range workflowFiles {
			fmt.Printf("  - %s (%d uses)\n", wf.Path, len(wf.UsesWithVersions))
		}
		fmt.Printf("Extracted %d unique action references\n", len(allActionRefs))
		if len(allActionRefs) > 0 {
			for _, ref := range allActionRefs {
				fmt.Printf("  - %s@%s\n", ref.OwnerRepo, ref.Version)
			}
		}
	}

	if len(allActionRefs) == 0 {
		fmt.Println("No GitHub Actions found in workflows")
		return
	}

	// Get unique owner/repo list for archived check
	ownerRepos := make([]string, 0, len(allActionRefs))
	seen := make(map[string]bool)
	for _, ref := range allActionRefs {
		if !seen[ref.OwnerRepo] {
			seen[ref.OwnerRepo] = true
			ownerRepos = append(ownerRepos, ref.OwnerRepo)
		}
	}

	// Check GitHub API for archived status
	fmt.Printf("Checking %d action repositories for archived status...\n", len(ownerRepos))

	archived, errors := ghClient.CheckMultipleRepos(ctx, ownerRepos)

	if verbose && len(errors) > 0 {
		fmt.Printf("API errors encountered:\n")
		for repo, err := range errors {
			fmt.Printf("  - %s: %v\n", repo, err)
		}
	}

	// Collect archived actions with workflow context
	var archivedActions []issue.ArchivedActionInfo
	var archivedRepos []string

	for _, wf := range workflowFiles {
		for _, ref := range wf.UsesWithVersions {
			if isArchived, exists := archived[ref.OwnerRepo]; exists && isArchived {
				archivedActions = append(archivedActions, issue.ArchivedActionInfo{
					Repo:     ref.OwnerRepo,
					Workflow: filepath.Base(wf.Path),
					Uses:     ref.FullRef,
				})
				archivedRepos = append(archivedRepos, ref.OwnerRepo)
			}
		}
	}

	// Remove duplicates from archivedRepos
	archivedRepos = removeDuplicates(archivedRepos)

	// Check for outdated actions if requested
	var outdatedActions []OutdatedActionInfo
	if checkOutdated {
		// Filter out archived repos for outdated check
		var nonArchivedRepos []string
		for _, ref := range allActionRefs {
			if isArchived, exists := archived[ref.OwnerRepo]; !exists || !isArchived {
				nonArchivedRepos = append(nonArchivedRepos, ref.OwnerRepo)
			}
		}
		nonArchivedRepos = removeDuplicates(nonArchivedRepos)

		if len(nonArchivedRepos) > 0 {
			fmt.Printf("Checking %d non-archived action repositories for latest versions...\n", len(nonArchivedRepos))
			releases, releaseErrors := ghClient.CheckMultipleReleases(ctx, nonArchivedRepos)

			if verbose && len(releaseErrors) > 0 {
				fmt.Printf("Release API errors encountered:\n")
				for repo, err := range releaseErrors {
					fmt.Printf("  - %s: %v\n", repo, err)
				}
			}

			// Check each action for outdated status
			for _, wf := range workflowFiles {
				for _, ref := range wf.UsesWithVersions {
					// Skip archived actions
					if isArchived, exists := archived[ref.OwnerRepo]; exists && isArchived {
						continue
					}

					release, hasRelease := releases[ref.OwnerRepo]
					if !hasRelease {
						continue
					}

					// Check if current ref is a major version tag and same major as latest
					if ver.IsMajorVersionTag(ref.Version) && ver.SameMajorVersion(ref.Version, release.TagName) {
						// Compare commit SHAs to determine if major version tag is up to date
						same, _, _, err := ghClient.CompareRefSHAs(ctx, ref.OwnerRepo, ref.Version, release.TagName)
						if err != nil {
							if verbose {
								fmt.Printf("  Cannot compare SHAs for %s@%s vs %s: %v\n", ref.OwnerRepo, ref.Version, release.TagName, err)
							}
							continue
						}
						// If SHAs are the same, the major version tag points to latest, so not outdated
						if same {
							continue
						}
						// SHAs differ, so the major version tag is outdated
						outdatedActions = append(outdatedActions, OutdatedActionInfo{
							OwnerRepo:  ref.OwnerRepo,
							CurrentRef: ref.Version,
							LatestTag:  release.TagName,
							LatestURL:  release.HTMLURL,
							Workflow:   filepath.Base(wf.Path),
							FullRef:    ref.FullRef,
						})
						continue
					}

					// Standard version comparison for non-major-version tags
					isOutdated, err := ver.IsVersionOutdated(ref.Version, release.TagName)
					if err != nil {
						if verbose {
							fmt.Printf("  Cannot compare versions for %s: %v\n", ref.OwnerRepo, err)
						}
						continue
					}

					if isOutdated {
						outdatedActions = append(outdatedActions, OutdatedActionInfo{
							OwnerRepo:  ref.OwnerRepo,
							CurrentRef: ref.Version,
							LatestTag:  release.TagName,
							LatestURL:  release.HTMLURL,
							Workflow:   filepath.Base(wf.Path),
							FullRef:    ref.FullRef,
						})
					}
				}
			}
		}
	}

	// Report findings
	hasIssues := len(archivedActions) > 0 || len(outdatedActions) > 0

	if !hasIssues {
		fmt.Println("✅ No archived or outdated GitHub Actions found!")
		return
	}

	// Report archived actions
	if len(archivedActions) > 0 {
		fmt.Printf("\n🚨 Found %d archived GitHub Actions in %d workflows:\n\n", len(archivedRepos), len(archivedActions))

		// Group by workflow
		workflowMap := make(map[string][]issue.ArchivedActionInfo)
		for _, action := range archivedActions {
			workflowMap[action.Workflow] = append(workflowMap[action.Workflow], action)
		}

		for wf, actions := range workflowMap {
			fmt.Printf("📄 %s:\n", wf)
			for _, action := range actions {
				fmt.Printf("  ❌ %s\n", action.Uses)
			}
			fmt.Println()
		}
	}

	// Report outdated actions
	if len(outdatedActions) > 0 {
		uniqueOutdated := make(map[string]bool)
		for _, action := range outdatedActions {
			uniqueOutdated[action.OwnerRepo] = true
		}

		fmt.Printf("\n⚠️  Found %d outdated GitHub Actions in %d uses:\n\n", len(uniqueOutdated), len(outdatedActions))

		// Group by workflow
		outdatedWorkflowMap := make(map[string][]OutdatedActionInfo)
		for _, action := range outdatedActions {
			outdatedWorkflowMap[action.Workflow] = append(outdatedWorkflowMap[action.Workflow], action)
		}

		for wf, actions := range outdatedWorkflowMap {
			fmt.Printf("📄 %s:\n", wf)
			for _, action := range actions {
				fmt.Printf("  ⚠️  %s@%s (latest: %s)\n", action.OwnerRepo, action.CurrentRef, action.LatestTag)
			}
			fmt.Println()
		}
	}

	// Get repository name for notifications/issues
	repoName := getRepoName(workDir)

	// Send notifications if enabled
	if notifier != nil && len(archivedActions) > 0 {
		// Convert to notification types
		var notificationActions []notification.ArchivedActionInfo
		for _, action := range archivedActions {
			notificationActions = append(notificationActions, notification.ArchivedActionInfo{
				Repo:     action.Repo,
				Workflow: action.Workflow,
				Uses:     action.Uses,
			})
		}
		if err := notifier.NotifyArchivedActions(ctx, notificationActions, repoName); err != nil {
			log.Errorf("Failed to send notifications: %v", err)
		}
	}

	// Create GitHub issue if enabled
	if issueCreator != nil && repoName != "" && len(archivedActions) > 0 {
		parts := strings.Split(repoName, "/")
		if len(parts) == 2 {
			owner, repo := parts[0], parts[1]
			if err := issueCreator.CreateArchivedActionIssue(ctx, owner, repo, archivedActions); err != nil {
				log.Errorf("Failed to create GitHub issue: %v", err)
			}
		}
	}

	// Exit with error code if issues found
	if len(archivedActions) > 0 {
		fmt.Println("\n❌ Archived actions detected. Please replace them with actively maintained alternatives.")
		os.Exit(1)
	} else if len(outdatedActions) > 0 {
		fmt.Println("\n⚠️  Outdated actions detected. Consider updating to the latest versions.")
		os.Exit(1)
	}
}

// OutdatedActionInfo contains information about an outdated action.
type OutdatedActionInfo struct {
	OwnerRepo  string
	CurrentRef string
	LatestTag  string
	LatestURL  string
	Workflow   string
	FullRef    string
}

// rootCmdPreRun performs setup operations before executing the root command.
func rootCmdPreRun(cmd *cobra.Command, args []string) {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
}

// Execute starts the command-line interface execution.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// init initializes the command-line interface during package loading.
func init() {
	// get configuration from environment variables
	conf = config.GetEnvVars()

	// persistent flags
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug-level logging")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed output")

	// command flags
	rootCmd.Flags().StringVarP(&workflowPath, "workflow", "w", "", "Path to specific workflow file to check")
	rootCmd.Flags().StringVarP(&githubToken, "token", "t", "", "GitHub token (overrides GH_TOKEN/GITHUB_TOKEN env vars)")
	rootCmd.Flags().BoolVar(&notify, "notify", false, "Send notifications to configured endpoints")
	rootCmd.Flags().BoolVar(&createIssue, "create-issue", false, "Create GitHub issue when archived actions found")
	rootCmd.Flags().BoolVar(&checkOutdated, "check-outdated", false, "Check for outdated action versions")

	// add sub-commands
	rootCmd.AddCommand(
		man.NewManCmd(),
		version.Command(),
	)
}

// removeDuplicates removes duplicate strings from a slice.
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string
	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}
	return result
}

// getRepoName attempts to determine the repository name from the git remote.
func getRepoName(workDir string) string {
	// This is a simple implementation - could be enhanced to parse git config
	// For now, we'll use a placeholder or try to extract from path
	return "current-repo" // TODO: Implement proper repo name detection
}
