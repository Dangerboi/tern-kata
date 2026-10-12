## Agent skills

### Issue tracker

Issues live in kata (project `tern-kata`), driven through the `kata` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary, applied as kata labels. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Secret scan

`.githooks/pre-commit` runs gitleaks on staged changes and blocks the commit if it finds a secret. Install it once per clone: `ln -sf ../../.githooks/pre-commit .git/hooks/pre-commit`. If it blocks a commit, remove the secret; never bypass it with `--no-verify`.
