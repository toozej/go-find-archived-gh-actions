# go-find-archived-gh-actions

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/toozej/go-find-archived-gh-actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/toozej/go-find-archived-gh-actions)](https://goreportcard.com/report/github.com/toozej/go-find-archived-gh-actions)
![GitHub Actions CI Workflow Status](https://img.shields.io/github/actions/workflow/status/toozej/go-find-archived-gh-actions/ci.yaml)
![GitHub Actions Release Workflow Status](https://img.shields.io/github/actions/workflow/status/toozej/go-find-archived-gh-actions/release.yaml)
![GitHub Actions Weekly Docker Refresh Workflow Status](https://img.shields.io/github/actions/workflow/status/toozej/go-find-archived-gh-actions/weekly-docker-refresh.yaml)
![Docker Pulls](https://img.shields.io/docker/pulls/toozej/go-find-archived-gh-actions)
![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/toozej/go-find-archived-gh-actions/total)

A tool to detect archived GitHub Actions in repository workflows.

## What it does

This tool scans your GitHub Actions workflows (`.github/workflows/**/*.yml` and `**/*.yaml`) and checks if any of the `uses:` actions have been archived by their maintainers on GitHub. Archived actions may contain security vulnerabilities, stop receiving updates, or cease working with future GitHub changes.

## Features

- 🔍 **Automatic Detection**: Scans all workflow files in your repository
- 🚨 **Exit Codes**: Returns error code when archived actions are found (CI/CD friendly)
- ⚠️ **Outdated Detection**: Optionally checks for outdated action versions
- 📢 **Notifications**: Send alerts to configured webhooks when archived actions are detected
- 🐛 **Issue Creation**: Automatically create GitHub issues to track replacement tasks
- 🔧 **Flexible Configuration**: Environment variables, config files, and CLI flags
- 📊 **Verbose Output**: Detailed reporting of findings and API calls
- 🐳 **Docker Support**: Run via Docker or as native binary

## Installation

### From GitHub Releases

Download the latest release from [GitHub Releases](https://github.com/toozej/go-find-archived-gh-actions/releases).

### Using Go

```bash
go install github.com/toozej/go-find-archived-gh-actions@latest
```

### Docker

```bash
docker run --rm ghcr.io/toozej/go-find-archived-gh-actions:latest
```

## Usage

### Basic Usage

```bash
# Check all workflows in current repository
go-find-archived-gh-actions

# Check a specific workflow file
go-find-archived-gh-actions --workflow .github/workflows/ci.yml

# Verbose output
go-find-archived-gh-actions --verbose

# Debug logging
go-find-archived-gh-actions --debug

# Check for outdated actions (not archived, but not latest version)
go-find-archived-gh-actions --check-outdated
```

### Authentication

Set your GitHub token using one of these methods (in order of priority):

1. `--token` flag
2. `GH_TOKEN` environment variable
3. `GITHUB_TOKEN` environment variable

```bash
# Using environment variable
export GH_TOKEN=your_github_token_here
go-find-archived-gh-actions

# Using CLI flag
go-find-archived-gh-actions --token your_github_token_here
```

### Notifications

Configure one or more notification providers and enable them with the `--notify` flag:

```bash
# Example: Configuring Slack
export SLACK_TOKEN=xoxb-...
export SLACK_CHANNEL_ID=C12345678
go-find-archived-gh-actions --notify
```

### Issue Creation

Automatically create GitHub issues when archived actions are found:

```bash
go-find-archived-gh-actions --create-issue
```

### Configuration File

Create a `.env` file in your repository root:

```env
GH_TOKEN=your_github_token_here
SLACK_TOKEN=xoxb-...
SLACK_CHANNEL_ID=C12345678
CREATE_ISSUES=true
```

### GitHub Action

Use the provided GitHub Action in your workflows:

```yaml
name: Check for Archived Actions
on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly
  workflow_dispatch:

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Check archived actions
        id: check
        uses: toozej/go-find-archived-gh-actions@main
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          verbose: true
          create-issue: true

      - name: Fail if archived actions found
        if: steps.check.outputs.has-archived == 'true'
        run: exit 1
```

### Pre-commit Hook

Add to your `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/toozej/go-find-archived-gh-actions
    rev: main
    hooks:
      - id: go-find-archived-gh-actions
        name: Check for archived GitHub Actions
        args: [--verbose]
```

## Exit Codes

- `0`: Success - no archived or outdated actions found
- `1`: Error - archived or outdated actions found, or execution failed

## Example Output

### Archived Actions Only

```
$ go-find-archived-gh-actions --verbose

Found 3 workflow files
  - .github/workflows/ci.yml (2 uses)
  - .github/workflows/release.yml (1 uses)
Extracted 3 unique action references
  - actions/checkout
  - actions/setup-go
  - docker/build-push-action

Checking 3 action repositories for archived status...

🚨 Found 1 archived GitHub Actions in 1 workflows:

📄 .github/workflows/ci.yml:
  ❌ actions/checkout

❌ Archived actions detected. Please replace them with actively maintained alternatives.
```

### With Outdated Checking

```
$ go-find-archived-gh-actions --verbose --check-outdated

Found 1 workflow files
  - example/workflows/example-archived-actions.yaml (9 uses)
Extracted 9 unique action references
  - actions-rs/toolchain@v1
  - actions/cache@v2
  - actions/checkout@v4
Checking 9 action repositories for archived status...
Checking 5 non-archived action repositories for latest versions...

🚨 Found 4 archived GitHub Actions in 4 workflows:

📄 example-archived-actions.yaml:
  ❌ actions-rs/audit-check@v1
  ❌ actions-rs/cargo@v1
  ❌ actions-rs/clippy-check@v1
  ❌ actions-rs/toolchain@v1


⚠️  Found 2 outdated GitHub Actions in 2 uses:

📄 example-archived-actions.yaml:
  ⚠️  actions/cache@v2 (latest: v4.0.0)
  ⚠️  actions/checkout@v4 (latest: v4.1.0)

❌ Archived actions detected. Please replace them with actively maintained alternatives.
```

### Major Version Tag Handling

When using `--check-outdated`, the tool intelligently handles major version tags (e.g., `v6`):

- If you're using `actions/checkout@v6` and the latest release is `v6.0.2`, the tool compares the commit SHAs
- If the major version tag (`v6`) points to the same commit as the latest patch version (`v6.0.2`), it's **not** marked as outdated
- This allows you to use major version tags (recommended practice) without false positives

```
# actions/checkout@v6 points to v6.0.2 (same SHA) - NOT outdated
# actions/cache@v2 but latest is v5.0.5 (different major) - IS outdated
```

## Configuration

### Core Settings

| Environment Variable | CLI Flag | Description |
|---------------------|----------|-------------|
| `GH_TOKEN` | `--token`, `-t` | GitHub API token (preferred) |
| `GITHUB_TOKEN` | `--token`, `-t` | GitHub API token (fallback) |
| `CREATE_ISSUES` | `--create-issue` | Create GitHub issues (true/false) |
| `NOTIFY_CONDENSE` | - | Condense multiple notifications into one (true/false) |
| - | `--notify` | Enable notifications to configured endpoints |
| - | `--workflow`, `-w` | Path to specific workflow file to check |
| - | `--check-outdated` | Check for outdated action versions |
| - | `--verbose`, `-v` | Show detailed output |
| - | `--debug`, `-d` | Enable debug-level logging |

### Notification Providers

Configure one or more of the following providers to receive alerts when archived actions are found. Use the `--notify` flag to enable notifications.

| Provider | Environment Variables |
|----------|-----------------------|
| **Gotify** | `GOTIFY_ENDPOINT`, `GOTIFY_TOKEN` |
| **Slack** | `SLACK_TOKEN`, `SLACK_CHANNEL_ID` |
| **Telegram** | `TELEGRAM_TOKEN`, `TELEGRAM_CHAT_ID` |
| **Discord** | `DISCORD_TOKEN`, `DISCORD_CHANNEL_ID` |
| **Pushover** | `PUSHOVER_TOKEN`, `PUSHOVER_RECIPIENT_ID` |
| **Pushbullet** | `PUSHBULLET_TOKEN`, `PUSHBULLET_DEVICE_NICKNAME` |

## Quick Demo

To quickly see how this tool works, run the example demo which checks an example workflow containing archived and outdated actions:

```bash
# Build and run against example workflow (includes outdated checking)
make example-demo

# Or run manually after building
make local-build
./out/go-find-archived-gh-actions --workflow example/workflows/example-archived-actions.yaml --verbose --check-outdated
```

The example workflow at `example/workflows/example-archived-actions.yaml` contains:
- **Archived actions** (4 from `actions-rs/*` organization): `actions-rs/toolchain`, `actions-rs/cargo`, `actions-rs/clippy-check`, `actions-rs/audit-check`
- **Current actions** (GitHub official): `actions/checkout@v6`, `actions/setup-go@v6`, `github/codeql-action`, `actions/upload-artifact@v4`, `actions/download-artifact@v4`
- **Outdated but not archived**: `actions/cache@v2` (latest: v5.x), `actions/download-artifact@v4` (latest: v8.x), `actions/upload-artifact@v4` (latest: v7.x)

