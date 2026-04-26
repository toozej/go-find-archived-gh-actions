package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkflowParser_FindWorkflowFiles(t *testing.T) {
	parser := NewParser()

	// Create temporary directory structure
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	err := os.MkdirAll(workflowsDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create workflows directory: %v", err)
	}

	// Create test workflow files
	testFiles := []string{
		"ci.yml",
		"release.yaml",
		"test.yml",
		"not-a-workflow.txt", // Should be ignored
	}

	for _, file := range testFiles {
		path := filepath.Join(workflowsDir, file)
		content := "name: test\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n"
		if file == "not-a-workflow.txt" {
			content = "not yaml"
		}
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", file, err)
		}
	}

	files, err := parser.FindWorkflowFiles(tmpDir)
	if err != nil {
		t.Fatalf("FindWorkflowFiles failed: %v", err)
	}

	expectedFiles := 3 // Only .yml and .yaml files
	if len(files) != expectedFiles {
		t.Errorf("Expected %d workflow files, got %d", expectedFiles, len(files))
	}

	// Check that all expected files are found
	expectedPaths := map[string]bool{
		filepath.Join(workflowsDir, "ci.yml"):       true,
		filepath.Join(workflowsDir, "release.yaml"): true,
		filepath.Join(workflowsDir, "test.yml"):     true,
	}

	for _, file := range files {
		if !expectedPaths[file] {
			t.Errorf("Unexpected file found: %s", file)
		}
	}
}

func TestWorkflowParser_ParseWorkflowFile(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		content  string
		expected []string
		hasError bool
	}{
		{
			name: "valid workflow with uses",
			content: `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go test ./...
      - uses: golangci/golangci-lint-action@v3`,
			expected: []string{"actions/checkout", "actions/setup-go", "golangci/golangci-lint-action"},
			hasError: false,
		},
		{
			name: "workflow without uses",
			content: `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "hello"`,
			expected: []string{},
			hasError: false,
		},
		{
			name: "invalid yaml",
			content: `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: invalid
        uses: actions/setup-go@v4
        invalid_field: [unclosed bracket
  another:
    runs-on: ubuntu-latest`,
			expected: []string{}, // Should fail to parse, so no uses extracted
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "workflow.yml")
			err := os.WriteFile(filePath, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			result, err := parser.ParseWorkflowFile(filePath)

			if tt.hasError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if result != nil {
				if result.Uses == nil && len(tt.expected) == 0 {
					// Both are effectively empty
				} else if !reflect.DeepEqual(result.Uses, tt.expected) {
					t.Errorf("Expected uses %v (len=%d), got %v (len=%d)", tt.expected, len(tt.expected), result.Uses, len(result.Uses))
				}
				if result.Path != filePath {
					t.Errorf("Expected path %s, got %s", filePath, result.Path)
				}
			}
		})
	}
}

func TestWorkflowParser_extractUsesFromYAML(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		yaml     string
		expected []string
	}{
		{
			name: "simple uses",
			yaml: `jobs:
  test:
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4`,
			expected: []string{"actions/checkout", "actions/setup-go"},
		},
		{
			name: "complex workflow structure",
			yaml: `jobs:
  test:
    steps:
      - uses: actions/checkout@v3
  build:
    steps:
      - uses: docker/build-push-action@v4
  matrix:
    strategy:
      matrix:
        go-version: [1.19, 1.20]
    steps:
      - uses: actions/setup-go@v4`,
			expected: []string{"actions/checkout", "actions/setup-go", "docker/build-push-action"},
		},
		{
			name: "duplicates removed",
			yaml: `jobs:
  test:
    steps:
      - uses: actions/checkout@v3
      - uses: actions/checkout@v3`,
			expected: []string{"actions/checkout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uses, err := parser.extractUsesFromYAML([]byte(tt.yaml))
			if err != nil {
				t.Errorf("extractUsesFromYAML failed: %v", err)
			}

			if !reflect.DeepEqual(uses, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, uses)
			}
		})
	}
}

func TestWorkflowParser_deduplicateAndClean(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no duplicates",
			input:    []string{"actions/checkout@v3", "actions/setup-go@v4"},
			expected: []string{"actions/checkout", "actions/setup-go"},
		},
		{
			name:     "with duplicates",
			input:    []string{"actions/checkout@v3", "actions/checkout@v3", "actions/setup-go@v4"},
			expected: []string{"actions/checkout", "actions/setup-go"},
		},
		{
			name:     "empty and invalid",
			input:    []string{"", "actions/checkout@v3", "invalid"},
			expected: []string{"actions/checkout"},
		},
		{
			name:     "sorted output",
			input:    []string{"docker/build-push-action@v4", "actions/checkout@v3", "actions/setup-go@v4"},
			expected: []string{"actions/checkout", "actions/setup-go", "docker/build-push-action"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.deduplicateAndClean(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWorkflowParser_deduplicateAndCleanWithVersions(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    []string
		expected []ActionRef
	}{
		{
			name:  "no duplicates",
			input: []string{"actions/checkout@v3", "actions/setup-go@v4"},
			expected: []ActionRef{
				{OwnerRepo: "actions/checkout", Version: "v3", FullRef: "actions/checkout@v3"},
				{OwnerRepo: "actions/setup-go", Version: "v4", FullRef: "actions/setup-go@v4"},
			},
		},
		{
			name:  "with duplicates - keeps first",
			input: []string{"actions/checkout@v3", "actions/checkout@v2", "actions/setup-go@v4"},
			expected: []ActionRef{
				{OwnerRepo: "actions/checkout", Version: "v3", FullRef: "actions/checkout@v3"},
				{OwnerRepo: "actions/setup-go", Version: "v4", FullRef: "actions/setup-go@v4"},
			},
		},
		{
			name:  "branch ref",
			input: []string{"actions/checkout@main", "actions/setup-go@master"},
			expected: []ActionRef{
				{OwnerRepo: "actions/checkout", Version: "main", FullRef: "actions/checkout@main"},
				{OwnerRepo: "actions/setup-go", Version: "master", FullRef: "actions/setup-go@master"},
			},
		},
		{
			name:  "commit sha",
			input: []string{"actions/checkout@abc123def456"},
			expected: []ActionRef{
				{OwnerRepo: "actions/checkout", Version: "abc123def456", FullRef: "actions/checkout@abc123def456"},
			},
		},
		{
			name:     "empty and invalid",
			input:    []string{"", "actions/checkout@v3", "invalid"},
			expected: []ActionRef{{OwnerRepo: "actions/checkout", Version: "v3", FullRef: "actions/checkout@v3"}},
		},
		{
			name:  "sorted output",
			input: []string{"docker/build-push-action@v4", "actions/checkout@v3", "actions/setup-go@v4"},
			expected: []ActionRef{
				{OwnerRepo: "actions/checkout", Version: "v3", FullRef: "actions/checkout@v3"},
				{OwnerRepo: "actions/setup-go", Version: "v4", FullRef: "actions/setup-go@v4"},
				{OwnerRepo: "docker/build-push-action", Version: "v4", FullRef: "docker/build-push-action@v4"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.deduplicateAndCleanWithVersions(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWorkflowParser_ParseWorkflowFiles(t *testing.T) {
	parser := NewParser()

	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.yml")
	file2 := filepath.Join(tmpDir, "file2.yml")

	if err := os.WriteFile(file1, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v3"), 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/setup-go@v4"), 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	paths := []string{file1, file2}
	results, err := parser.ParseWorkflowFiles(paths)

	if err != nil {
		t.Fatalf("ParseWorkflowFiles failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 workflow files, got %d", len(results))
	}
}

func TestWorkflowParser_GetAllUsesFromRepo(t *testing.T) {
	parser := NewParser()

	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("Failed to create workflows directory: %v", err)
	}

	file1 := filepath.Join(workflowsDir, "ci.yml")
	file2 := filepath.Join(workflowsDir, "release.yml")

	if err := os.WriteFile(file1, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v3"), 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v3\n      - uses: actions/setup-go@v4"), 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	uses, files, err := parser.GetAllUsesFromRepo(tmpDir)

	if err != nil {
		t.Fatalf("GetAllUsesFromRepo failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	expectedUses := []string{"actions/checkout", "actions/setup-go"}
	if !reflect.DeepEqual(uses, expectedUses) {
		t.Errorf("Expected uses %v, got %v", expectedUses, uses)
	}

	// Test error scenarios
	workflows, _ := parser.ParseWorkflowFiles([]string{"nonexistent.yml"})
	if len(workflows) != 1 || workflows[0].Error == nil {
		t.Error("Expected error inside WorkflowFile for nonexistent file")
	}

	uses_all, files, err := parser.GetAllUsesFromRepo("/path/that/does/not/exist/surely")
	if err != nil {
		t.Errorf("Expected nil error from GetAllUsesFromRepo for nonexistent directory, got %v", err)
	}
	if len(uses_all) != 0 || len(files) != 0 {
		t.Errorf("Expected 0 uses and files, got %d and %d", len(uses_all), len(files))
	}
}
