# Issue tracker: kata

Issues and specs for this repo live in kata, project `tern-kata` (bound by `.kata.toml`). Use the CLI with `--agent` for reads; run `kata quickstart` once per session for the full agent contract.

## Conventions

- A spec is an issue labelled `spec`; its body is the spec text.
- Implementation tickets are child issues of the spec: `kata create "<title>" --parent <spec-ref> --body-file <path>`.
- Triage state is a label (see `triage-labels.md`); swap with `kata label rm` / `kata label add`.
- Conversation goes in comments: `kata comment <ref> --body "..."`.
- Close only with evidence: `kata close <ref> --done --message "..." [--commit <sha>]`.

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
- **Claim**: `kata claim <ref> --if-unowned` before any work.
- **Resolve**: comment the answer, `kata close <ref> --done --message "<gist>"`, then append the gist and ref to the map's Decisions-so-far.
