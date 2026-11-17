// Package whatsnew cuts the release notes a newly started version shows out of tern-kata's CHANGELOG.md.
package whatsnew

import (
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// section is one release's part of the changelog: its heading without the brackets ("0.17.0 - 2026-11-20") and its lines.
type section struct {
	version, heading string
	lines            []string
}

// releaseHeading is a released version's heading, "## [0.17.0] - 2026-11-20"; [Unreleased] is not one.
var releaseHeading = regexp.MustCompile(`^## \[(\d+\.\d+\.\d+)\](.*)$`)

// sections are the released versions' sections in the changelog's order, newest first. A section ends at the next "## " heading
// or at the link references the file ends with.
func sections(changelog string) []section {
	var all []section
	var current *section
	for line := range strings.SplitSeq(changelog, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "[") && strings.Contains(line, "]: ") {
			current = nil
			if m := releaseHeading.FindStringSubmatch(line); m != nil {
				all = append(all, section{version: "v" + m[1], heading: m[1] + m[2]})
				current = &all[len(all)-1]
			}
			continue
		}
		if current != nil {
			current.lines = append(current.lines, line)
		}
	}
	for i := range all {
		all[i].lines = trimBlank(all[i].lines)
	}
	return all
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// breaking is every top-level "- **Breaking:**" bullet of s with its sub-bullets, each led by the version.
func (s section) breaking() []string {
	var out []string
	in := false
	for _, line := range s.lines {
		switch {
		case strings.HasPrefix(line, "- **Breaking:**"):
			in = true
			out = append(out, "- "+strings.TrimPrefix(s.version, "v")+": "+strings.TrimPrefix(line, "- "))
		case in && strings.HasPrefix(line, "  "):
			out = append(out, line)
		default:
			in = false
		}
	}
	return out
}

// Notes is the Markdown the first Pane of version on this machine shows, where shown is the version shown last ("" when none was,
// or it cannot be read). It holds the sections of the versions after shown up to version, separate and newest first, titled
// "tern-kata <shown> → <version>"; across several versions every Breaking bullet is listed first too, as those ask the reader to
// act. With no version shown before, it is only version's own section, titled "What's new in tern-kata <version>". Versions are
// written with their v in the titles and without it in the section headings, as the changelog writes them. ok is false when there
// is nothing to show: version has no section of its own (a dev build), or it is not newer than shown.
func Notes(changelog, shown, version string) (string, bool) {
	if !semver.IsValid(shown) {
		shown = ""
	}
	if shown != "" && semver.Compare(version, shown) <= 0 {
		return "", false
	}
	var picked []section
	own := false
	for _, s := range sections(changelog) {
		if s.version == version {
			own = true
		}
		after := shown == "" && s.version == version || shown != "" && semver.Compare(s.version, shown) > 0
		if after && semver.Compare(s.version, version) <= 0 {
			picked = append(picked, s)
		}
	}
	if !own {
		return "", false
	}
	var b strings.Builder
	if shown == "" {
		b.WriteString("# What's new in tern-kata " + version + "\n")
	} else {
		b.WriteString("# tern-kata " + shown + " → " + version + "\n")
	}
	if len(picked) > 1 {
		var breaking []string
		for _, s := range picked {
			breaking = append(breaking, s.breaking()...)
		}
		if len(breaking) > 0 {
			b.WriteString("\n## Breaking changes\n\n" + strings.Join(breaking, "\n") + "\n")
		}
	}
	for _, s := range picked {
		b.WriteString("\n## " + s.heading + "\n\n" + strings.Join(s.lines, "\n") + "\n")
	}
	return b.String(), true
}
