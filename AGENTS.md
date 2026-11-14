## Agent skills

### Issue tracker

Issues live in kata (project `tern-kata`), driven through the `kata` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary, applied as kata labels. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Attachments

Files on Issues are kata's native Docbank attachments: reading them and attaching evidence is `kata docbank attachments`, in `skills/kata/SKILL.md`. `skills/tern-kata/SKILL.md` covers the old forms, the `tern-kata.attachment.<name>` keys and, from 0.3, the `attachment.<name>` keys holding `{host, path}`, and their migration.

## Checks

Commands, smoke options and the Windows dev install: `docs/guide/development.md`.

- Before each commit: `scripts/check.sh static` (about 40 s, ends with `CHECKS ok`).
- Go tests for the packages you changed (cached unless the code changed; full runs without `-count=1`, about 35 s uncached).
- After a UI change: the smoke steps that cover it, plus every step naming an id, class or text the diff renames or removes (`grep -rl <id> smoke/steps`), each as `./smoke/run.sh smoke/steps/NN-name.sh` (5-15 s).
- The full `./smoke/run.sh` once per branch (3-6 min; the push gate runs it).
- Stop only the smoke servers you started; other agents run theirs beside yours.
- Before you ask the user to try a change, merge it and run `scripts/dev-install.sh` from the primary checkout: it installs the build here and on the Windows window machine, which runs its own plugin copy.

## Changelog

Every user-visible change adds its line under `## [Unreleased]` in `CHANGELOG.md`, in the commit that closes its spec or ticket. Entry rules and release steps: `docs/agents/changelog.md`.
