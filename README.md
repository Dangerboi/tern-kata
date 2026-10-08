# tern-kata

Native kata Issues in a Tern Pane. Requires Go 1.27+, kata v0.18.0+ on `PATH`, and Tern with Surface Protocol support. Without Tern support, arguments pass unchanged to `kata tui`.

```sh
CGO_ENABLED=0 go build -o tern-kata .
CGO_ENABLED=0 go install github.com/Dangerboi/tern-kata@latest
CGO_ENABLED=0 go test ./...
./smoke/run.sh
```

Run `tern-kata` inside Tern in a directory bound with `kata init`. Use `j`/`k`, `g`/`G`, and `q`/`ctrl+c`. Press `?` for all bindings with their modes; `?` or `esc` closes help without moving the selected Issue. The dock shows only current-mode hints, and the help layer lists every binding, both from `model/keymap.go`.

The Collections sidebar loads Inbox, Today, Upcoming, Deadlines, All Open, Logbook and Projects from the daemon UI snapshot. Unbound directories open Inbox. Press `esc` from the Issue list to focus Collections, navigate with `j`/`k` or `g`/`G`, and press `enter` to switch; sidebar items are also clickable. Date headings match kata's web UI: literal calendar dates for Upcoming and Logbook, Overdue/Today for due work, with today's evening Issues merged into Today. Snapshot requests include the local calendar date and IANA time zone and reuse ETags only for the same Collection intent.

The smoke runner needs `kata`, `tern` and `jq` on `PATH`. It isolates both instances, seeds three open Issues in Project `smoke`, and runs `smoke/steps/*` in filename order: `.ctl` scenarios or sourced `.sh` assertions using `ctl` and `expect_tree`. Add a step file for each feature; leave the app in the Issue list for the next step. Use `plugins expect` for native text, but `expect_tree` for list labels (Tern's text search excludes them). Failed runs retain their temporary artifacts and exit non-zero.

Launch commands in smoke steps through a `.ctl` file containing `run "<command>"`; passing an unquoted command argument directly to `tern ctl run` loses the headless runner's required script quoting.

Views use Tern's native typography, spacing and selection accent for readable Issue rows, following the host theme without a palette or added animation (energy/rhythm/motion 1/1/1). Model Msg/Cmd values are extended in per-feature files; `model/keymap.go` remains the single source for key handling and hints.

Issue lists refresh from kata's durable event stream, resuming after the last applied snapshot cursor. A history reset refetches the complete Collection. Live merges keep Issue UIDs and selection stable so Tern retains native list scroll; they do not reveal the selection again. Disconnected streams keep stale Issues visible read-only while reconnecting with capped exponential backoff. An unreachable daemon shows kata's message and a clickable **Retry daemon** button (`ctrl+r`).
