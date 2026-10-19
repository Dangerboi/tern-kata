#!/bin/sh
# Prints the release notes for a tag: its CHANGELOG.md section, without the heading. Fails when there is none.
# Usage: scripts/release-notes.sh vX.Y.Z
set -eu

tag=${1:?usage: scripts/release-notes.sh vX.Y.Z}
root=$(cd "$(dirname "$0")/.." && pwd)

# The section runs from "## [X.Y.Z]" to the next version heading or the link references at the end.
notes=$(awk -v head="## [${tag#v}]" '
	index($0, head) == 1 { found = 1; next }
	found && (/^## \[/ || /^\[[^]]*\]: /) { exit }
	found && !started && /^[[:space:]]*$/ { next }
	found { started = 1; print }
' "$root/CHANGELOG.md")

if [ -z "$notes" ]; then
	echo "CHANGELOG.md has no section for $tag: rename [Unreleased] to [${tag#v}] before tagging" >&2
	exit 1
fi
printf '%s\n' "$notes"
