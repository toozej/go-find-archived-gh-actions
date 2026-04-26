// Package workflow provides functionality for parsing GitHub Actions workflow files
// and extracting GitHub Action references.
//
// This package handles finding workflow files, parsing YAML content, and extracting
// all 'uses:' references from jobs and steps.
package workflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ActionRef represents a GitHub Action reference with owner/repo and version.
type ActionRef struct {
	OwnerRepo string // e.g., "actions/checkout"
	Version   string // e.g., "v3", "main", "abc123"
	FullRef   string // e.g., "actions/checkout@v3"
}

// WorkflowFile represents a GitHub Actions workflow file.
type WorkflowFile struct {
	Path             string
	Uses             []string    // Legacy: just owner/repo names
	UsesWithVersions []ActionRef // New: includes version info
	Error            error
}

// WorkflowParser handles parsing of GitHub Actions workflow files.
type WorkflowParser struct{}

// NewParser creates a new WorkflowParser instance.
func NewParser() *WorkflowParser {
	return &WorkflowParser{}
}

// FindWorkflowFiles finds all GitHub Actions workflow files in the repository.
// It looks for .github/workflows/**/*.yml and .github/workflows/**/*.yaml files.
func (p *WorkflowParser) FindWorkflowFiles(rootDir string) ([]string, error) {
	var workflowFiles []string

	workflowsDir := filepath.Join(rootDir, ".github", "workflows")
	if _, err := os.Stat(workflowsDir); os.IsNotExist(err) {
		return workflowFiles, nil // No workflows directory, return empty
	}

	err := filepath.Walk(workflowsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			workflowFiles = append(workflowFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk workflows directory: %w", err)
	}

	return workflowFiles, nil
}

// ParseWorkflowFile parses a single workflow file and extracts all 'uses:' references.
func (p *WorkflowParser) ParseWorkflowFile(filePath string) (*WorkflowFile, error) {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)

	root, err := os.OpenRoot(dir)
	if err != nil {
		return &WorkflowFile{Path: filePath, Error: err}, err
	}
	defer root.Close()

	f, err := root.Open(base)
	if err != nil {
		return &WorkflowFile{Path: filePath, Error: err}, err
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return &WorkflowFile{Path: filePath, Error: err}, err
	}

	uses, err := p.extractUsesFromYAML(content)
	if err != nil {
		return &WorkflowFile{Path: filePath, Uses: uses, Error: err}, err
	}

	usesWithVersions, err := p.extractUsesFromYAMLWithVersions(content)
	if err != nil {
		return &WorkflowFile{Path: filePath, Uses: uses, Error: err}, err
	}

	return &WorkflowFile{Path: filePath, Uses: uses, UsesWithVersions: usesWithVersions}, nil
}

// ParseWorkflowFiles parses multiple workflow files and returns their parsed results.
func (p *WorkflowParser) ParseWorkflowFiles(filePaths []string) ([]*WorkflowFile, error) {
	var results []*WorkflowFile

	for _, path := range filePaths {
		workflow, err := p.ParseWorkflowFile(path)
		results = append(results, workflow)
		if err != nil {
			// Continue parsing other files even if one fails
			continue
		}
	}

	return results, nil
}

// extractUsesFromYAML parses YAML content and extracts all 'uses:' references.
// It handles both simple string values and complex action references with @version.
func (p *WorkflowParser) extractUsesFromYAML(content []byte) ([]string, error) {
	var workflow map[string]interface{}
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var uses []string
	p.extractUsesRecursive(workflow, &uses)

	// Deduplicate and clean up uses
	uses = p.deduplicateAndClean(uses)

	return uses, nil
}

// extractUsesFromYAMLWithVersions parses YAML content and extracts all 'uses:' references with versions.
func (p *WorkflowParser) extractUsesFromYAMLWithVersions(content []byte) ([]ActionRef, error) {
	var workflow map[string]interface{}
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	var uses []string
	p.extractUsesRecursive(workflow, &uses)

	// Deduplicate and clean up uses with version info
	actionRefs := p.deduplicateAndCleanWithVersions(uses)

	return actionRefs, nil
}

// extractUsesRecursive recursively walks through the YAML structure to find 'uses' keys.
func (p *WorkflowParser) extractUsesRecursive(data interface{}, uses *[]string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if key == "uses" {
				if str, ok := value.(string); ok {
					*uses = append(*uses, str)
				}
			} else {
				p.extractUsesRecursive(value, uses)
			}
		}
	case []interface{}:
		for _, item := range v {
			p.extractUsesRecursive(item, uses)
		}
	}
}

// deduplicateAndClean removes duplicates and cleans up uses references.
// It extracts owner/repo from uses strings like "actions/checkout@v3".
func (p *WorkflowParser) deduplicateAndClean(uses []string) []string {
	seen := make(map[string]bool)
	cleaned := make([]string, 0) // Ensure non-nil slice

	// Regex to match owner/repo@ref patterns
	re := regexp.MustCompile(`^([^/@]+/[^/@]+)@`)

	for _, use := range uses {
		use = strings.TrimSpace(use)
		if use == "" {
			continue
		}

		// Extract owner/repo part
		matches := re.FindStringSubmatch(use)
		if len(matches) > 1 {
			repo := matches[1]
			if !seen[repo] {
				seen[repo] = true
				cleaned = append(cleaned, repo)
			}
		}
	}

	sort.Strings(cleaned)
	return cleaned
}

// deduplicateAndCleanWithVersions removes duplicates and extracts owner/repo and version.
// It returns ActionRef structs with both owner/repo and version info.
func (p *WorkflowParser) deduplicateAndCleanWithVersions(uses []string) []ActionRef {
	seen := make(map[string]bool)
	actionRefs := make([]ActionRef, 0)

	// Regex to match owner/repo@ref patterns
	re := regexp.MustCompile(`^([^/@]+/[^/@]+)@(.+)$`)

	for _, use := range uses {
		use = strings.TrimSpace(use)
		if use == "" {
			continue
		}

		// Extract owner/repo and version parts
		matches := re.FindStringSubmatch(use)
		if len(matches) > 2 {
			ownerRepo := matches[1]
			version := matches[2]

			// Deduplicate by owner/repo (keep first occurrence)
			if !seen[ownerRepo] {
				seen[ownerRepo] = true
				actionRefs = append(actionRefs, ActionRef{
					OwnerRepo: ownerRepo,
					Version:   version,
					FullRef:   use,
				})
			}
		}
	}

	// Sort by owner/repo
	sort.Slice(actionRefs, func(i, j int) bool {
		return actionRefs[i].OwnerRepo < actionRefs[j].OwnerRepo
	})

	return actionRefs
}

// GetAllUsesFromRepo finds all workflow files in a repository and extracts unique uses references.
func (p *WorkflowParser) GetAllUsesFromRepo(rootDir string) ([]string, []*WorkflowFile, error) {
	files, err := p.FindWorkflowFiles(rootDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workflow files: %w", err)
	}

	workflows, err := p.ParseWorkflowFiles(files)
	if err != nil {
		return nil, workflows, fmt.Errorf("failed to parse workflow files: %w", err)
	}

	var allUses []string
	seen := make(map[string]bool)

	for _, workflow := range workflows {
		for _, use := range workflow.Uses {
			if !seen[use] {
				seen[use] = true
				allUses = append(allUses, use)
			}
		}
	}

	sort.Strings(allUses)
	return allUses, workflows, nil
}

// GetAllUsesFromRepoWithVersions finds all workflow files and extracts unique uses with version info.
func (p *WorkflowParser) GetAllUsesFromRepoWithVersions(rootDir string) ([]ActionRef, []*WorkflowFile, error) {
	files, err := p.FindWorkflowFiles(rootDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workflow files: %w", err)
	}

	workflows, err := p.ParseWorkflowFiles(files)
	if err != nil {
		return nil, workflows, fmt.Errorf("failed to parse workflow files: %w", err)
	}

	var allActionRefs []ActionRef
	seen := make(map[string]bool)

	for _, workflow := range workflows {
		for _, actionRef := range workflow.UsesWithVersions {
			if !seen[actionRef.OwnerRepo] {
				seen[actionRef.OwnerRepo] = true
				allActionRefs = append(allActionRefs, actionRef)
			}
		}
	}

	// Sort by owner/repo
	sort.Slice(allActionRefs, func(i, j int) bool {
		return allActionRefs[i].OwnerRepo < allActionRefs[j].OwnerRepo
	})

	return allActionRefs, workflows, nil
}
