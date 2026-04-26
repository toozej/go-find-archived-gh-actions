package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRemoveDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string(nil),
		},
		{
			name:     "no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "all duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeDuplicates(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("removeDuplicates() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetRepoName(t *testing.T) {
	originalEnv := os.Getenv("GITHUB_REPOSITORY")
	defer os.Setenv("GITHUB_REPOSITORY", originalEnv)

	os.Unsetenv("GITHUB_REPOSITORY")
	result := getRepoName("/some/fake/path")
	expected := "current-repo"
	if result != expected {
		t.Errorf("getRepoName() = %v, want %v", result, expected)
	}

	os.Setenv("GITHUB_REPOSITORY", "owner/repo")
	result = getRepoName("/some/fake/path")
	expected = "owner/repo"
	if result != expected {
		t.Errorf("getRepoName() = %v, want %v", result, expected)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home directory: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		workDir  string
		expected string
	}{
		{
			name:     "tilde expansion",
			path:     "~/some/path",
			workDir:  "/some/workdir",
			expected: filepath.Join(home, "some/path"),
		},
		{
			name:     "tilde with subdirectory",
			path:     "~/src/github/repo/.github/workflows",
			workDir:  "/current/dir",
			expected: filepath.Join(home, "src/github/repo/.github/workflows"),
		},
		{
			name:     "absolute path unchanged",
			path:     "/absolute/path/to/dir",
			workDir:  "/some/workdir",
			expected: "/absolute/path/to/dir",
		},
		{
			name:     "relative path joined with workDir",
			path:     "relative/path",
			workDir:  "/base/dir",
			expected: "/base/dir/relative/path",
		},
		{
			name:     "simple relative path",
			path:     ".github/workflows",
			workDir:  "/home/user/repo",
			expected: "/home/user/repo/.github/workflows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.path, tt.workDir)
			if result != tt.expected {
				t.Errorf("expandPath(%q, %q) = %q, want %q", tt.path, tt.workDir, result, tt.expected)
			}
		})
	}
}
