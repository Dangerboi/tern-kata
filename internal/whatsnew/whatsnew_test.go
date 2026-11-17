package whatsnew_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/whatsnew"
)

const changelog = `# Changelog

Intro.

## [Unreleased]

### Added

- Not released yet.

## [0.17.0] - 2026-11-20

### Removed

- **Breaking:** The old widget is gone.
  - Remove it from the bar.
- Something else went.

## [0.16.0] - 2026-11-18

### Added

- A new card.

## [0.15.0] - 2026-11-16

### Fixed

- **Breaking:** A flag was renamed.

[Unreleased]: https://example.invalid/compare/v0.17.0...HEAD
[0.17.0]: https://example.invalid/compare/v0.16.0...v0.17.0
`

func TestSeveralVersionsAreSeparateNewestFirstWithTheBreakingLinesFirst(t *testing.T) {
	got, ok := whatsnew.Notes(changelog, "v0.15.0", "v0.17.0")
	want := `# tern-kata v0.15.0 → v0.17.0

## Breaking changes

- 0.17.0: **Breaking:** The old widget is gone.
  - Remove it from the bar.

## 0.17.0 - 2026-11-20

### Removed

- **Breaking:** The old widget is gone.
  - Remove it from the bar.
- Something else went.

## 0.16.0 - 2026-11-18

### Added

- A new card.
`
	if !ok || got != want {
		t.Fatalf("notes (ok %v):\n%s\nwant:\n%s", ok, got, want)
	}
}

func TestWithNoVersionShownBeforeOnlyThisVersionsSectionShows(t *testing.T) {
	got, ok := whatsnew.Notes(changelog, "", "v0.16.0")
	want := `# What's new in tern-kata v0.16.0

## 0.16.0 - 2026-11-18

### Added

- A new card.
`
	if !ok || got != want {
		t.Fatalf("notes (ok %v):\n%s\nwant:\n%s", ok, got, want)
	}
}

// A build whose version has no section of its own (a dev build, or one stamped from a commit between releases) shows nothing, nor
// does a version already shown or an older one.
func TestNothingToShow(t *testing.T) {
	for name, tc := range map[string]struct{ shown, version string }{
		"dev build":         {"v0.15.0", "dev"},
		"stamped dev build": {"v0.15.0", "v0.17.0-3-gabc1234-dev"},
		"already shown":     {"v0.17.0", "v0.17.0"},
		"older than shown":  {"v0.17.0", "v0.16.0"},
		"no section":        {"v0.15.0", "v0.18.0"},
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := whatsnew.Notes(changelog, tc.shown, tc.version); ok {
				t.Errorf("notes for %s after %s:\n%s", tc.version, tc.shown, got)
			}
		})
	}
}
