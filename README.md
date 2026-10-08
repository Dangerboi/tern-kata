# tern-kata

Native kata Issues in a Tern Pane. Requires Go 1.27+, kata v0.18.0+ on `PATH`, and Tern with Surface Protocol support. Without Tern support, compatible arguments pass to `kata tui`; `--as` becomes `KATA_AUTHOR` because kata's TUI ignores the flag.

```sh
CGO_ENABLED=0 go build -o tern-kata .
CGO_ENABLED=0 go install github.com/Dangerboi/tern-kata@latest
CGO_ENABLED=0 go test ./...
./smoke/run.sh
```

Run `tern-kata` inside Tern in a directory bound with `kata init`. Use `j`/`k`, `g`/`G`, and `q`/`ctrl+c`. Press `?` for all bindings with their modes; `?` or `esc` closes help without moving the selected Issue. The dock shows only current-mode hints, and the help layer lists every binding, both from `model/keymap.go`.

Selecting an Issue loads its Markdown body, properties, labels, links, comments and change history beside the list. `enter` opens detail, `esc` returns to the list (then Collections), `tab`/`shift+tab` cycle detail sections, and `enter` follows the selected referenced Issue; `esc` unwinds those jumps. `L` hides or restores detail. Panes narrower than 100 columns show only the focused column.

In the list or detail, `+` adds a label, `-` removes one, `a` assigns an owner, `A` clears the owner, and `!` sets priority. Prompts save with `enter` (or `ctrl+o`/`ctrl+s`) and cancel with `esc`; priority accepts 0-4 or `-` to clear. Labels complete with up/down and tab using project labels for addition and attached labels for removal. Rejected writes keep the prompt and kata's error visible. Writes use `--as <actor>`, then `KATA_AUTHOR`, `USER`, git user.name, or anonymous, in that order; authenticated kata identities retain authority over the requested actor.
Issue rows show short id, title, status, priority, owner and labels. Nested lists start collapsed: `space` toggles the selected parent's children, right/left expands/collapses, `E` toggles every parent, and `v` switches flat/nested (returning to nested collapses all). Expansion follows Issue UIDs across refreshes; collapsing moves a hidden selection to its nearest visible ancestor. Priority 0 uses the host's error tone, and closed Issues use its muted tone.
The Collections sidebar loads Inbox, Today, Upcoming, Deadlines, All Open, Logbook and Projects from the daemon UI snapshot. Unbound directories open Inbox. Press `esc` from the Issue list to focus Collections, navigate with `j`/`k` or `g`/`G`, and press `enter` to switch; sidebar items are also clickable. Date headings match kata's web UI: literal calendar dates for Upcoming and Logbook, Overdue/Today for due work, with today's evening Issues merged into Today. Snapshot requests include the local calendar date and IANA time zone and reuse ETags only for the same Collection intent.

`/` searches the current Collection through the daemon snapshot as you type. Up/down moves through live results; `enter` first focuses results and then commits the search. In results, `esc` or `/` returns to the query; query `esc` restores the previous search and selected Issue. `s` cycles all/open/closed without a ready state. `f` opens Project, status, owner, label and relationship filters: tab/shift+tab changes fields, left/right or space cycles choices, and ctrl+o (or ctrl+s) applies; Escape cancels. Owner matches exactly, comma-separated labels match any label, and relationships are Has parent, Has child, Blocks, Blocked by or Related. All axes combine, remain visible in the list header, and survive event refreshes and Collection changes. Project filters cannot widen a Project Collection. `c` clears every filter and restores the Collection's default status; explicit all status includes both open and closed results. The form also has Apply filters and Cancel buttons.

Press `x` in the Issue list or detail to open the close form. Use `tab`/`shift+tab` to select a field, `ctrl+u` to clear it, `ctrl+o` to submit, or `esc` to cancel. Commit SHA, PR URL and test command evidence are optional fields, but normal `done` requires at least one; duplicate/superseded and audit reasons have dedicated reference/rationale fields. **Quick done** (`ctrl+d`) sends the owner-local interactive exemption, not fabricated evidence. The daemon still enforces parent-close and throttle guards. Close uses the loaded revision and `close-v1`; conflict, guard and throttle messages remain readable in the form. Press `r` to reopen.

The smoke runner needs `kata`, `tern` and `jq` on `PATH`. It isolates both instances, seeds three open Issues in Project `smoke`, and runs `smoke/steps/*` in filename order: `.ctl` scenarios or sourced `.sh` assertions using `ctl` and `expect_tree`. Add a step file for each feature; leave the app in the Issue list for the next step. Use `plugins expect` for native text, but `expect_tree` for list labels (Tern's text search excludes them). Failed runs retain their temporary artifacts and exit non-zero.

Launch commands in smoke steps through a `.ctl` file containing `run "<command>"`; passing an unquoted command argument directly to `tern ctl run` loses the headless runner's required script quoting.

Views use Tern's native typography, spacing and selection accent for readable Issue rows, following the host theme without a palette or added animation (energy/rhythm/motion 1/1/1). Model Msg/Cmd values are extended in per-feature files; `model/keymap.go` remains the single source for key handling and hints.

Issue lists refresh from kata's durable event stream, resuming after the last applied snapshot cursor. A history reset refetches the complete Collection. Live merges keep Issue UIDs and selection stable so Tern retains native list scroll; they do not reveal the selection again. Disconnected streams keep stale Issues visible read-only while reconnecting with capped exponential backoff. An unreachable daemon shows kata's message and a clickable **Retry daemon** button (`ctrl+r`).
