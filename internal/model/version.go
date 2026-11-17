package model

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Semver is version as semver, with the leading v ("0.9.26" and "v0.9.26-dev" both name a Tern), "" when it is no version.
func Semver(version string) string {
	if v := "v" + strings.TrimPrefix(strings.TrimSpace(version), "v"); semver.IsValid(v) {
		return v
	}
	return ""
}

// TernAtLeast reports whether a Tern of this version is at least floor, a semver with a leading v. A pre-release is older than its
// release, and a version that cannot be read is not at least anything.
func TernAtLeast(version, floor string) bool {
	v := Semver(version)
	return v != "" && semver.Compare(v, floor) >= 0
}
