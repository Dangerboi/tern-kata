# Changelog

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Its sections are the release notes: the release workflow publishes the tag's section through `scripts/release-notes.sh`, and a tag whose version has no section fails to release.

## Adding an entry

- **When:** in the commit that closes a spec, or a ticket outside a spec, whose change a user can notice. The line goes under `## [Unreleased]`, in Added, Changed, Fixed or Removed; add the subsection when it is missing.
- **Reader:** someone who installs and uses tern-kata. Write one line per change they would notice, in the present tense, starting with the thing that changed and using GLOSSARY terms ("Logbook days use your local time zone").
- **A spec** gets the lines its Solution section implies, not one per child ticket. Refactors, tests, formatting and CI changes get none.
- **Name the behaviour**: the tracker links tickets and commits, so entries carry neither ids nor hashes.
- **Superseded:** when later work replaces something already under Unreleased, rewrite that entry in its final form.

## Releasing vX.Y.Z

1. Rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and put a new empty `## [Unreleased]` above it. At the end of the file, point `[Unreleased]` at `compare/vX.Y.Z...HEAD` and add `[X.Y.Z]` comparing the previous tag with `vX.Y.Z`.
2. Set `version = "X.Y.Z"` in `plugin.toml`.
3. Run `scripts/release-notes.sh vX.Y.Z` to read the notes and `scripts/release.sh vX.Y.Z` to build; both fail when the changelog or `plugin.toml` does not match the tag.
4. Commit, tag `vX.Y.Z` and push the tag. The release workflow publishes.
