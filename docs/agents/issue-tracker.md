# Issue tracker: kata

Issues and specs for this repo live in kata, project `tern-kata` (bound by `.kata.toml`). Use the CLI with `--agent` for reads; run `kata quickstart` once per session for the full agent contract.

## Conventions

- A spec is an issue labelled `spec`; its body is the spec text.
- Implementation tickets are child issues of the spec: `kata create "<title>" --parent <spec-ref> --body-file <path>`.
- Triage state is a label (see `triage-labels.md`); swap with `kata label rm` / `kata label add`.
- Conversation goes in comments: `kata comment <ref> -m "..."`. Answer a prior comment with `--reply <comment-ref>`, `--confirm <comment-ref>` (verified or reproduced), `--refute <comment-ref>` or `--supersede <comment-ref>`; `--confirm` and `--refute` need at least 40 characters of evidence.
- Retried writes (`create`, `comment`, `close`) pass `--idempotency-key <key>`, so a repeat returns the first result instead of a duplicate.
- Close only with evidence: `kata close <ref> --done -m "..." [--commit <sha>]`. The message must be at least 40 characters for `--done` and `--audit-no-change`, 20 for `--duplicate-of` and `--superseded-by`, 60 for `--wontfix`.
- Setup or connectivity problems: run `kata doctor`. Install the agent hooks with `kata agent-hook install`.

## When a skill says "publish to the issue tracker"

`kata search "<title>" --agent` first; if nothing matches, `kata create` with `--label`/`--parent` as needed. Report the short id it prints.

## When a skill says "fetch the relevant ticket"

`kata show <ref> --agent`. Refs are a short id (`abc4`), `project#abc4`, or a ULID.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a parent issue with one **child** issue per ticket.

- **Map**: an issue labelled `map`; its body holds Notes / Decisions-so-far / Fog. Update it with `kata edit <map> --body`.
- **Child ticket**: `kata create "<question>" --parent <map> --label type:<research|prototype|grilling|task>`.
- **Blocking**: `--blocked-by <ref>` on create or edit.
- **Frontier**: `kata ready --unowned --agent`, filtered to the map's children; first listed wins.
- **Claim**: `kata claim <ref> --if-unowned --ttl <duration>` (1m to 24h, e.g. `2h`) before any work; an abandoned claim returns to the frontier when the TTL lapses.
- **Resolve**: comment the answer, `kata close <ref> --done -m "<gist>"`, then append the gist and ref to the map's Decisions-so-far.
