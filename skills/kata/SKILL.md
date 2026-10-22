---
name: kata
description: "Kata issue tracker CLI. Use when a repo has `.kata.toml` or names Kata as its tracker: finding, creating, claiming, commenting, replying, labelling or closing issues (tickets), making work recur, scheduling agent jobs, being run by a tern-kata worker, or coordinating subagents through issues."
---

Kata is a daemon-backed issue ledger shared by humans and agents. Written against kata v0.19; `kata <cmd> --help` is the source of truth for flags, `kata quickstart --format contract` for upstream agent policy. Repo specifics (project name, extra labels, human gates) live in that repo's `AGENTS.md`.

## Output mode

- `--agent` on every ordinary read and mutation: one `OK <cmd> …` line plus `key=value` rows. Cheap and greppable.
- `--json` only when a later command consumes the result (e.g. capture `.issue.short_id` after `create`). Pipe through `jq`.
- Human output (no flag) is for the user's terminal, never for parsing.

## Refs

An issue ref is a short id (`m0h6`), a qualified short id (`myproject#m0h6`), or a full ULID. Qualify it when crossing projects. `kata status <ref> --agent` is the cheapest identity/hold check.

## Guardrails

- `delete` and `purge` run only when the user asks for that exact action on that exact ref. Both demand `--force --confirm "DELETE <qualified-id>"` / `"PURGE <qualified-id>"`; `delete` is reversible with `kata restore`, `purge` is not.
- Close only work that is done and verified (see Close). Incomplete work gets `needs-review` plus a comment.
- No practice, example, or scratch issues: the ledger is shared state.
- Search before creating; reuse an open issue when one fits.

## Find work

```bash
kata next --unowned --agent                      # single highest-priority ready issue
kata ready --unowned --label bug --no-label blocked --agent
kata list --agent                                # open issues, capped at 200 rows
kata list --status all --label needs-triage --max-priority 1 --agent
kata search "bracket orphan" --agent             # title/body/comments; --status open|closed
kata show <ref> --agent                          # issue + comments
kata labels --agent                              # label counts in project
```

- `ready`/`next` exclude issues with an open `blocked-by` predecessor, a future `scheduled_on`, or `someday=true`. `list` excludes none of these.
- Priority is `0..4`, **0 = highest**. `--label` repeats with AND logic; `--no-label` excludes.
- `--all` on `list`/`ready`/`next` spans every non-archived project.
- `list` stops at 200 rows (`--include-total` reports the full count). For every match, add `--sort created --json` and repeat with `--cursor <next_cursor>` until `complete` is true; `search --lexical` pages the same way. `--priority-unset` finds unprioritised issues.

## Create

```bash
kata create "<title>" --body-file /tmp/body.md --label bug --priority 1 --agent
kata create "<title>" --body-stdin --agent <<'EOF'
## Problem
...
EOF
```

- Body via `--body`, `--body-file`, or `--body-stdin`. Use a file or stdin for anything multi-line or containing quotes/backticks.
- A title resembling an existing issue soft-blocks the create; check the suggested match, and pass `--force-new` only if it really is different work.
- `--meta key=value` (repeatable) stores the value as a JSON string.
- `--idempotency-key <key>` makes retries and fan-out creates safe; reusing a key returns the original issue. `comment` and `close` take one too.
- Relationships at creation: `--parent`, `--blocks`, `--blocked-by`, `--related` (see Relationships).

## Edit, comment, label

```bash
kata edit <ref> --title "<t>" --priority 2 --agent        # --priority - clears
kata edit <ref> --body "$(cat /tmp/body.md)" --agent      # replaces the whole body
kata comment <ref> --body-file /tmp/c.md --agent          # -m/--body, --body-stdin also work
kata comment edit <ref> <comment-uid-or-handle> --body-file /tmp/c.md
kata label add <ref> <label> --agent
kata label rm <ref> <label> --agent
```

Gotchas:
- `kata edit` has **no** `--body-file`/`--body-stdin` and **no** `--label`. Labels go through `kata label add/rm`.
- `edit --body` replaces the body; read it with `show` first and send the full new text.
- `assign`, `claim`, `unassign`, `reopen`, `label add/rm`, `edit`, `close`, `move` accept `--comment "<text>"` to attach a comment in the same call. Prefer it over a second `comment` call.
- Concurrency: `close`, `meta set/unset`, `schedule` and `deadline` accept `--if-match <revision>` (`revision=` from `kata status <ref> --agent`); `edit` has none. The revision moves on owner and open/closed changes, not on title, body or label edits, so it catches another agent's claim or close, not a concurrent text edit.

## Reply to a comment

```bash
kata comment <ref> --reply c:kkkdm5 -m "<response>" --agent
kata comment <ref> --confirm c:kkkdm5 -m "<40+ chars: how you reproduced or verified it>" --agent
kata comment <ref> --refute c:kkkdm5 -m "<40+ chars of contrary evidence>" --agent
kata comment <ref> --supersede c:kkkdm5 -m "<the finding that replaces it>" --agent
kata show <ref> --thread c:kkkdm5 --agent     # that comment and its replies across the project, max 50
kata show <ref> --inbound=<actor> --agent     # replies to <actor>'s comments on this issue; bare --inbound = you
```

- Copy the handle from `handle=` on the comment's row in `kata show --agent`: `c:<suffix>` on this issue, `<issue>:<suffix>` for another issue in the project. Replies across projects are refused; use `--related`. The replying issue must be open.
- The kind is your assertion, not something kata checks: `reply` responds, `confirm` reproduced or verified it, `refute` contradicts it, `supersede` marks it outdated. Confirm and refute need 40+ characters after whitespace collapses.
- In `show --agent`, a `reply - kind=…` row under a comment names the comment it answers; `backlink - kind=…` rows name the replies it received.
- A second reply of the same kind to the same comment fails (exit 5) and names the existing one; `--force` only for a deliberate second assertion.
- `--inbound` takes its value with `=`; `--inbound alice` reads `alice` as a second argument and fails.
- Replies don't reach the inbox in v0.19.0 (kata's docs describe that as planned). To find answers to your findings, run `show --inbound`.

## Claim and live state

```bash
kata claim <ref> --if-unowned --agent              # fails if someone owns it; --force steals
kata claim <ref> --if-unowned --ttl 2h --agent     # timed: releases itself after 1m–24h
kata claim <ref> --ttl 2h --agent                  # renew your timed claim; --if-unowned would refuse it
kata unassign <ref> --expect-owner <you> --agent   # hand back only while it is still yours
kata meta set <ref> work.attention ok
kata meta set <ref> work.branch <branch>           # once, if work has its own branch
kata meta set <ref> work.attention stuck           # or needs-human
kata meta set <ref> work.attention_msg "<one-line why>"
kata meta get <ref> [key]
kata meta unset <ref> <key>
```

- `work.attention`: `ok` while working, `stuck` when you cannot proceed, `needs-human` when you want input (you may keep working). Reset to `ok` when unblocked. Always pair a non-ok value with `work.attention_msg`.
- End every session with either a close or a truthful `work.attention`; stale `ok` on abandoned work misleads coordinators.
- `meta set` values are strings; add `--json-value` for raw JSON (`kata meta set <ref> someday true --json-value`). Conditional writes: `--if-absent`, `--if-value <v>`.
- Prefer `--ttl` for queue work: if the session dies, the issue returns to `ready`/`next` when the claim expires. `kata status <ref> --agent` shows `assignment_expires_on`; `kata assign <ref> <you>` makes the claim permanent.
- `kata whoami` shows the resolved actor (`$KATA_AUTHOR` > `$USER` > git). `--as <actor>` overrides per call.

## Close

```bash
kata close <ref> --done --message "<scope and how it was verified>" \
  --commit <sha> --test "uv run pytest tests/test_x.py" --agent
kata close <ref> --wontfix --message "<why>" --agent
kata close <ref> --duplicate-of <ref> --message "<why>" --agent
kata close <ref> --superseded-by <ref> --message "<why>" --agent
kata close <ref> --audit-no-change --evidence "no-change-audit:<what was checked>" --message "<…>"
kata close <ref> --done ... --dry                    # validate without mutating
```

- A close is a completion claim. `--message` must be issue-specific prose: what changed, how it was verified.
- Evidence is typed: `--commit <sha>`, `--pr <url>`, `--test <cmd>`, `--reviewed <path>`, or raw `--evidence commit:|pr:|test:|reviewed-paths:|external:|no-change-audit:|duplicate-of:|superseded-by:<value>`. Only `--evidence` and `--reviewed` repeat: a second `--commit`/`--pr`/`--test` silently replaces the first, so cite several tests as one `--evidence "test:<cmd>"` each. `external:<account>` covers done work with no repo artifact. Non-wontfix closes without evidence show up in `kata audit closes --no-evidence`.
- Close each issue as soon as it is verified, not batched at the end.
- A parent cannot close while it has open children.
- Only `--done` makes a recurring issue's next occurrence (see Recurring work).
- Not finished: `kata label add <ref> needs-review --comment "<attempted / remains>"`. Never self-close a `needs-review` issue.
- `kata reopen <ref> --comment "<why>"` undoes a close.

## Attachments

An Issue's `attachment.<name>` meta keys are files attached through tern-kata: to read one, or to attach a screenshot, log or other file, use the `tern-kata` skill.

## Relationships

| Link | Flag | Meaning |
|---|---|---|
| parent | `--parent <ref>` (≤1; `edit --parent` replaces, `--remove-parent <current>` removes) | Containment and roll-up only. Does **not** gate `ready`. |
| blocks / blocked-by | `--blocks <dependent>`, `--blocked-by <prerequisite>` (`--remove-*` to drop) | Real prerequisite. Gates `ready`/`next` until the predecessor closes. |
| related | `--related <ref>` (`--remove-related`) | Symmetric context link, no ordering. |

Add blocking links only for genuine prerequisites; every one hides work from `ready`.

## Park and schedule

```bash
kata schedule <ref> 2026-11-01        # hidden from ready/next until then; '-' clears
kata meta set <ref> someday true --json-value   # undated parking; kata meta unset <ref> someday
kata deadline <ref> 2026-11-15        # sets deadline_on; never gates ready/next; '-' clears
```

- Dates take `YYYY-MM-DD`, local `YYYY-MM-DDTHH:MM`, or RFC 3339 ending in `Z`; relative words like `monday` are rejected. When the date arrives, the owner (or the author, if unowned) gets an inbox request.

## Recurring work

Two mechanisms; pick by what should happen:

| Want | Use |
|---|---|
| The same issue comes back after it is done, on a calendar rule | A **recurrence**. kata has no recurrence CLI: agents use `tern-kata recur` (below); people use `kata ui <ref>` (web UI) or tern-kata's `O` card. |
| A new child issue plus an inbox request every period, done or not | A **native cron job** bound to a parent (below). |

Recurrences:
- Make, change, stop or print one with `tern-kata recur`, which needs the `tern-kata` binary on `PATH`: check with `command -v tern-kata` (PowerShell: `Get-Command tern-kata`), and when it is missing tell the user and point them to https://github.com/Dangerboi/tern-kata#install instead of handing the job back. See the `tern-kata` skill for its flags. Leave `O` and `kata ui` to people.
- Each occurrence is an ordinary issue with `.issue.recurrence_id` and `.issue.occurrence_key` in `kata show --json`. Closing it `--done` creates the next one; `--wontfix`, `--duplicate-of` and `--superseded-by` create none, so the series stalls.
- Stopping a recurrence keeps existing occurrences. Occurrences can't be `kata move`d.

Native cron job:
```bash
cat > /tmp/job.json <<'EOF'
{"name": "Weekly dependency review",
 "definition": {"version": 1, "kind": "job", "enabled": true,
  "trigger": {"kind": "cron", "cron": "0 9 * * 1", "timezone": "UTC"},
  "action": {"kind": "notify", "recipient": "current-owner-or-author"},
  "issue": {"kind": "per-run", "title": "Review dependencies", "body": "<what to do each time>"},
  "overlap": "allow", "catchup": "latest"}}
EOF
kata cron job create --parent <ref> --file /tmp/job.json --json   # prints the job uid; reuse it via --uid on retry
kata cron job list --json
kata cron job update <job-uid> --parent <ref> --expected-event-uid <uid> --file /tmp/job.json --json   # whole definition again; without --parent it fails
```
- The parent stays open as the standing commitment. Each run adds a child titled with a UTC timestamp and the parent's current owner, plus an inbox request to that owner (or the parent's author). Clear it with `kata notify <child> --to <addr> --clear`.
- Triggers: `cron` (5 fields or `@daily`, IANA `timezone`) or `interval` (`interval_seconds` ≥ 60). `overlap` must be `allow`. `catchup`: `all` backfills every missed run, including runs missed while paused; `latest` only the newest; `skip` needs `grace_seconds`.
- The daemon delivers, so it must be running. Pause by setting `enabled` to false or closing the parent. `update`, `delete` and `restore` need the current `.job.definition_event_uid` from `kata cron job show <job-uid> --json`. Full format: https://www.katatracker.com/docs/reference/cron/#recurring-child-issues

### Jobs the tern-kata worker runs

Any other job (`action.kind` `execute`, or `notify` on an existing Issue or an Issue date) is for an adapter; kata never runs it. The user's `tern-kata worker` runs `execute` jobs with a prompt through omp, in a checkout, on the Projects listed in its `worker.toml`. Define one like this (`kata cron job create --file`, without `--parent`):
```json
{"name": "Review open PRs",
 "definition": {"version": 1, "kind": "job", "enabled": true,
  "trigger": {"kind": "cron", "cron": "0 9 * * 1-5", "timezone": "Europe/Oslo"},
  "action": {"kind": "execute", "prompt": "Review the open pull requests and record what needs a person."},
  "issue": {"kind": "per-run", "title": "PR review", "body": "<context for the run>"},
  "checkout_key": "tern-kata", "options": {"profile": "default"},
  "secret_refs": {"GITHUB_TOKEN": "github-review"},
  "overlap": "forbid", "catchup": "latest", "timeout_seconds": 3600}}
```
- `checkout_key` and `options.profile` name entries of the user's `worker.toml` (`[checkouts]`, `[profiles.<name>]`; the profile defaults to `default`). Never write a path or a secret into the definition: `secret_refs` maps an environment variable name to a `[secrets]` name the user defined. If the name is not in their `worker.toml`, tell them what to add.
- `issue` is either `per-run` (a new Issue per run, titled `<title> — <UTC occurrence>`) or `{"kind": "existing", "uid": "<ULID>"}` (the run only comments on that open Issue).
- The worker runs a job whoever wrote it, so write only what the user asked for. Native jobs (`notify` with a per-run Issue and a parent) stay with kata's daemon.
- The user can run one now with `tern-kata run <job-uid>` (see the `tern-kata` skill).

### If you are a run the worker started

The prompt that started you names your Issue (`<project>#<short id>`), the job, the run uid and the occurrence, then the task, then a contract. You run as the worker's actor (`KATA_AUTHOR`), in the job's checkout, headless with no one to answer questions. When you finish, the worker records your run in kata; you do not report it yourself. What you must do:
- Comment on your Issue what you did and how you checked it: `kata comment <ref> -m "..."`.
- When the work is done and verified, close the Issue with evidence: `kata close <ref> --done -m "<what was done and how it was verified>"` plus `--commit <sha>`, `--pr <url>`, `--test "<command>"` or `--reviewed <path>`; when nothing needed changing, `kata close <ref> --audit-no-change -m "<what you checked>" --evidence "no-change-audit:<what was checked>"`. Never close it when the work is not done, and never invent evidence.
- On an existing Issue the job reports to (the prompt says so), only comment: never close, label or edit it.
- Your final message is the run's summary in kata: say plainly what happened, and say so when the task failed or needs a person. Exit status 0 with a finished turn is `succeeded`; anything else is `failed`.
- If you leave the Issue open, the worker comments your final message on it and labels it `needs-review`. If your process dies, the worker holds the run and never sends the prompt again.
- Do not run `tern-kata worker` or `tern-kata run` from inside a run.

## Fan-out / join (coordinating subagents)

Ownership rules:
- Pick queue work with `kata claim <ref> --if-unowned`. On conflict (exit 5) take the next issue; never `--force`.
- The coordinator owns every issue it delegates. Workers get the ref in their task and claim only their own child issue; only the owner writes `work.*`.
- Before yielding, a worker closes its issue or sets `work.attention` + `work.attention_msg` to the hand-off state.

1. Create one child per slice, capturing refs:
   ```bash
   ref=$(kata create "<slice>" --parent <epic> --meta work.branch=<branch> \
         --idempotency-key <epic>-<slice> --body-file /tmp/slice.md --json | jq -r .issue.short_id)
   ```
2. Hand each child its ref. The child claims it and owns its `work.*` keys; the coordinator only reads them (one writer per key).
3. Join:
   ```bash
   kata wait <ref1> <ref2> --until attention --any --timeout 300s --json
   ```
   - `--until closed` (default) | `attention` (any non-ok) | `needs-human` | `stuck`. A close also satisfies attention modes; `results[].reason` says `closed` or `attention`.
   - `--all` (default) waits for every ref; `--any` returns on the first.
   - Timeout exits with code **8** and lists `pending`. `--timeout 0` (default) waits forever, so always set one in automation.
   - `wait` is read-only and needs no dependency edge.
4. Attribution for spawned agents: env vars don't reach omp subagents, so give each its name in the task and have it pass `--teammate <name>` on every kata write (stamps comments / `metadata.teammate` on creates). Its inbox is `kata inbox --for <actor>/<name>`.

## Attention requests (inbox)

```bash
kata notify <ref> --to <actor>[/<teammate>] --message "<reason>"
kata inbox --for <actor>[/<teammate>] --agent
kata inbox --for <actor>[/<teammate>] --all --agent   # every project on the daemon; omit --project/--workspace
kata notify <ref> --to <actor>[/<teammate>] --clear      # after handling
```

Reading doesn't clear a request.

## History and audit

```bash
kata digest --since 24h               # changelog grouped by actor
kata audit closes --since <RFC3339> --no-evidence
kata events --after <cursor> --limit 100 --json   # poll with a saved cursor; reset cache on reset_required
```

## Setup (rare)

- `kata init --project <name>` writes committed `.kata.toml` and gitignores `.kata.local.toml` (per-machine overrides). `--with-agents` injects generic guidance into `AGENTS.md`/`CLAUDE.md`; prefer this skill plus a short repo-specific section instead.
- `kata move <ref> <project> --dry` previews moving an issue; links survive, the short id changes.
- `kata agent-hook install [--local]` installs the contract and attention hooks for detected coding agents (the plural `agent-hooks` is gone).
- `kata doctor` (read-only; never starts or repairs a daemon), then `kata health` / `kata daemon`, when commands fail to reach the daemon.
