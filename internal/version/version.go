package version

import (
	"math"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// OutdatedInfo contains information about an outdated action.
type OutdatedInfo struct {
	OwnerRepo    string
	CurrentRef   string
	LatestTag    string
	LatestURL    string
	IsOutdated   bool
	CompareError error
}

// SHAGetter is an interface for fetching commit SHAs for refs.
type SHAGetter interface {
	GetRefSHA(ctx interface{}, ownerRepo, ref string) (string, error)
	CompareRefSHAs(ctx interface{}, ownerRepo, ref1, ref2 string) (bool, string, string, error)
}

// IsVersionOutdated checks if the current version ref is outdated compared to the latest release tag.
// It handles various version formats:
// - Semantic versions: v1.2.3, v1, v2
// - Branch names: main, master
// - Commit SHAs: abc123def456 (not compared, assumed current)
func IsVersionOutdated(currentRef, latestTag string) (bool, error) {
	// Normalize references
	currentRef = strings.TrimSpace(currentRef)
	latestTag = strings.TrimSpace(latestTag)

	// If they're identical, not outdated
	if currentRef == latestTag {
		return false, nil
	}

	// Check if current ref looks like a commit SHA (7+ hex chars)
	if isCommitSHA(currentRef) {
		// Can't compare commit SHAs to tags, assume current
		return false, nil
	}

	// Check if current ref is a branch name
	if isBranchName(currentRef) {
		// Branch names are always "current" for rolling releases
		return false, nil
	}

	// Try semantic version comparison
	currentSemver, err := parseSemver(currentRef)
	if err != nil {
		// Can't parse as semver, can't determine if outdated
		return false, err
	}

	latestSemver, err := parseSemver(latestTag)
	if err != nil {
		// Can't parse latest as semver
		return false, err
	}

	// Compare versions
	return currentSemver.LessThan(latestSemver), nil
}

// IsMajorVersionTag checks if a ref is a major version tag (e.g., "v1", "v2").
func IsMajorVersionTag(ref string) bool {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "v")
	return regexp.MustCompile(`^\d+$`).MatchString(ref)
}

// GetMajorVersion extracts the major version number from a version string.
// Returns -1 if it cannot be determined.
func GetMajorVersion(version string) int64 {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")

	v, err := semver.NewVersion(version)
	if err != nil {
		return -1
	}

	major := v.Major()
	if major > math.MaxInt64 {
		return -1
	}
	return int64(major)
}

// SameMajorVersion checks if two versions have the same major version number.
func SameMajorVersion(v1, v2 string) bool {
	major1 := GetMajorVersion(v1)
	major2 := GetMajorVersion(v2)

	if major1 < 0 || major2 < 0 {
		return false
	}

	return major1 == major2
}

// parseSemver parses a version string into a semver.Version.
// Handles both "v1.2.3" and "1.2.3" formats.
// Also handles major-only versions like "v1" or "v2".
func parseSemver(version string) (*semver.Version, error) {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Handle major-only versions (e.g., "v1" -> "1.0.0")
	if regexp.MustCompile(`^\d+$`).MatchString(version) {
		version += ".0.0"
	}

	// Handle major.minor versions (e.g., "v1.2" -> "1.2.0")
	if regexp.MustCompile(`^\d+\.\d+$`).MatchString(version) {
		version += ".0"
	}

	return semver.NewVersion(version)
}

// isCommitSHA checks if a string looks like a Git commit SHA.
func isCommitSHA(s string) bool {
	// Match 7-40 hex characters (short to full SHA)
	matched, _ := regexp.MatchString(`^[a-fA-F0-9]{7,40}$`, s)
	return matched
}

// isBranchName checks if a string is likely a branch name.
func isBranchName(s string) bool {
	commonBranches := []string{"main", "master", "develop", "dev", "staging", "production", "prod"}
	sLower := strings.ToLower(s)
	for _, branch := range commonBranches {
		if sLower == branch {
			return true
		}
	}
	return false
}
