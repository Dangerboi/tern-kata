## Agent skills

### Issue tracker

Issues live in kata (project `tern-kata`), driven through the `kata` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary, applied as kata labels. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Attachments

Files on Issues are kata's native Docbank attachments: reading them and attaching evidence is `kata docbank attachments`, in `skills/kata/SKILL.md`. `skills/tern-kata/SKILL.md` covers the old `tern-kata.attachment.*` keys and their migration.

## Changelog

Every user-visible change adds its line under `## [Unreleased]` in `CHANGELOG.md`, in the commit that closes its spec or ticket. Entry rules and release steps: `docs/agents/changelog.md`.
