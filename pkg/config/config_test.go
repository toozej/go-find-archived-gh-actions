package config

import (
	"os"
	"path/filepath"
	"testing"
)

// notificationEnvVars lists every env var that belongs to NotificationConfig,
// used to ensure a clean slate before each sub-test.
var notificationEnvVars = []string{
	"GOTIFY_ENDPOINT", "GOTIFY_TOKEN",
	"SLACK_TOKEN", "SLACK_CHANNEL_ID",
	"TELEGRAM_TOKEN", "TELEGRAM_CHAT_ID",
	"DISCORD_TOKEN", "DISCORD_CHANNEL_ID",
	"PUSHOVER_TOKEN", "PUSHOVER_RECIPIENT_ID",
	"PUSHBULLET_TOKEN", "PUSHBULLET_DEVICE_NICKNAME",
	"NOTIFY_CONDENSE",
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name               string
		mockEnv            map[string]string
		mockEnvFile        string
		expectGitHubToken  string
		expectNotification NotificationConfig
		expectCreateIssues bool
		expectError        bool
	}{
		{
			name: "Valid GitHub token from GH_TOKEN",
			mockEnv: map[string]string{
				"GH_TOKEN": "gh_test_token",
			},
			expectGitHubToken:  "gh_test_token",
			expectNotification: NotificationConfig{},
			expectCreateIssues: false,
		},
		{
			name: "Valid GitHub token from GITHUB_TOKEN",
			mockEnv: map[string]string{
				"GITHUB_TOKEN": "github_test_token",
			},
			expectGitHubToken:  "github_test_token",
			expectNotification: NotificationConfig{},
			expectCreateIssues: false,
		},
		{
			name: "GH_TOKEN overrides GITHUB_TOKEN",
			mockEnv: map[string]string{
				"GH_TOKEN":     "gh_priority_token",
				"GITHUB_TOKEN": "github_lower_token",
			},
			expectGitHubToken:  "gh_priority_token",
			expectNotification: NotificationConfig{},
			expectCreateIssues: false,
		},
		{
			name: "Gotify notification config from env",
			mockEnv: map[string]string{
				"GOTIFY_ENDPOINT": "https://gotify.example.com",
				"GOTIFY_TOKEN":    "mytoken",
			},
			expectGitHubToken: "",
			expectNotification: NotificationConfig{
				GotifyEndpoint: "https://gotify.example.com",
				GotifyToken:    "mytoken",
			},
			expectCreateIssues: false,
		},
		{
			name: "Condense flag from env",
			mockEnv: map[string]string{
				"NOTIFY_CONDENSE": "true",
			},
			expectGitHubToken: "",
			expectNotification: NotificationConfig{
				Condense: true,
			},
			expectCreateIssues: false,
		},
		{
			name: "Create issues enabled",
			mockEnv: map[string]string{
				"CREATE_ISSUES": "true",
			},
			expectGitHubToken:  "",
			expectNotification: NotificationConfig{},
			expectCreateIssues: true,
		},
		{
			name:               "No environment variables or .env file",
			expectGitHubToken:  "",
			expectNotification: NotificationConfig{},
			expectCreateIssues: false,
		},
		{
			name: "Environment variables override .env file",
			mockEnv: map[string]string{
				"GH_TOKEN": "env_override_token",
			},
			mockEnvFile:        "GH_TOKEN=file_token\n",
			expectGitHubToken:  "env_override_token",
			expectNotification: NotificationConfig{},
			expectCreateIssues: false,
		},
		{
			name:              "Valid .env file with Slack config",
			mockEnvFile:       "GH_TOKEN=envfile_token\nSLACK_TOKEN=slack_tok\nSLACK_CHANNEL_ID=C999\nCREATE_ISSUES=true\n",
			expectGitHubToken: "envfile_token",
			expectNotification: NotificationConfig{
				SlackToken:     "slack_tok",
				SlackChannelID: "C999",
			},
			expectCreateIssues: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get current directory: %v", err)
			}

			// Save and restore all relevant env vars
			allKeys := append([]string{"GH_TOKEN", "GITHUB_TOKEN", "CREATE_ISSUES"}, notificationEnvVars...)
			originalEnv := make(map[string]string, len(allKeys))
			for _, k := range allKeys {
				originalEnv[k] = os.Getenv(k)
			}
			defer func() {
				for key, value := range originalEnv {
					if value != "" {
						os.Setenv(key, value)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Clean slate
			for _, k := range allKeys {
				os.Unsetenv(k)
			}

			tmpDir := t.TempDir()
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("Failed to change to temp directory: %v", err)
			}
			defer func() {
				if err := os.Chdir(originalDir); err != nil {
					t.Errorf("Failed to restore original directory: %v", err)
				}
			}()

			if tt.mockEnvFile != "" {
				envPath := filepath.Join(tmpDir, ".env")
				if err := os.WriteFile(envPath, []byte(tt.mockEnvFile), 0644); err != nil {
					t.Fatalf("Failed to write mock .env file: %v", err)
				}
			}

			for key, value := range tt.mockEnv {
				os.Setenv(key, value)
			}

			conf, err := loadConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if conf.GitHubToken != tt.expectGitHubToken {
				t.Errorf("expected GitHubToken %q, got %q", tt.expectGitHubToken, conf.GitHubToken)
			}
			if conf.Notification.GotifyEndpoint != tt.expectNotification.GotifyEndpoint {
				t.Errorf("expected GotifyEndpoint %q, got %q", tt.expectNotification.GotifyEndpoint, conf.Notification.GotifyEndpoint)
			}
			if conf.Notification.GotifyToken != tt.expectNotification.GotifyToken {
				t.Errorf("expected GotifyToken %q, got %q", tt.expectNotification.GotifyToken, conf.Notification.GotifyToken)
			}
			if conf.Notification.SlackToken != tt.expectNotification.SlackToken {
				t.Errorf("expected SlackToken %q, got %q", tt.expectNotification.SlackToken, conf.Notification.SlackToken)
			}
			if conf.Notification.SlackChannelID != tt.expectNotification.SlackChannelID {
				t.Errorf("expected SlackChannelID %q, got %q", tt.expectNotification.SlackChannelID, conf.Notification.SlackChannelID)
			}
			if conf.Notification.Condense != tt.expectNotification.Condense {
				t.Errorf("expected Condense %v, got %v", tt.expectNotification.Condense, conf.Notification.Condense)
			}
			if conf.CreateIssues != tt.expectCreateIssues {
				t.Errorf("expected CreateIssues %v, got %v", tt.expectCreateIssues, conf.CreateIssues)
			}
		})
	}
}

func TestGetEnvVars(t *testing.T) {
	// Test success path
	os.Setenv("GH_TOKEN", "success_token")
	defer os.Unsetenv("GH_TOKEN")
	conf := GetEnvVars()
	if conf.GitHubToken != "success_token" {
		t.Errorf("Expected success_token, got %s", conf.GitHubToken)
	}

	// Test error path — force env.Parse to fail with an invalid int for TELEGRAM_CHAT_ID
	os.Setenv("TELEGRAM_CHAT_ID", "not-a-number")
	defer os.Unsetenv("TELEGRAM_CHAT_ID")

	exitCalled := false
	originalOsExit := osExit
	osExit = func(code int) {
		exitCalled = true
		if code != 1 {
			t.Errorf("Expected exit code 1, got %d", code)
		}
	}
	defer func() {
		osExit = originalOsExit
	}()

	GetEnvVars()

	if !exitCalled {
		t.Error("Expected osExit to be called")
	}
}
