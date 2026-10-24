# Talk to the kata daemon; never embed kata

tern-kata reads and writes kata only through the running daemon's HTTP API, using the typed client `go.kenn.io/kata/pkg/client`. It does not embed `go.kenn.io/kata`, even though that module is importable and would remove the daemon dependency.

Embedding constructs a `kata.Service` that owns its own storage handle. Running one beside the daemon that the CLI, agents and `kata tui` already use would mean two writers on one database, or a second database nobody else sees. kata's architecture treats the daemon's HTTP service as the single access path, so every client sees the same validation, actor attribution, event ordering and hooks.

## Consequences

- A daemon must be running; tern-kata never starts one, with exactly one exception below. We find it with kata's own `client.Discover` for the workspace, which picks the daemon and its credentials and handles Unix sockets. With no reachable daemon the Pane shows kata's message plus a hint to start the daemon or run `kata doctor`. The daemon's kind (local or remote) comes from `Client.Discovered()`.
- The exception: when the user deliberately switches the Pane to a local entry of the daemon catalog on the Daemons Page and that daemon is stopped, the Daemon gateway starts it, as `kata tui` does, by running `kata --daemon <name> daemon recover` for a profile (an entry with a `home`), which builds the profile's own environment and refuses a profile whose storage is gone or whose instance is not the pinned one rather than creating an empty database, or `kata daemon start` in the current home for an entry without one, with `KATA_DSN`, `KATA_DB` and `KATA_SERVER` removed from the child's environment, as kata's documentation asks when a home is administered. Nothing else starts a daemon: not launching the Pane, not a reconnect, not a failed read, not a remote entry.
- The daemon catalog (`active_daemon`, the `[[daemon]]` entries and `[tui].confirm_quit`) is read straight from `<KATA_HOME>/config.toml` by one interim module in the data package, with kata v0.20's defaults and validation, because kata's client does not expose it yet. A missing file is an empty catalog; an unreadable one is an error naming the path. Replace the module once `pkg/client` ships the catalog.
- Live updates come from the daemon's durable event stream, not in-process callbacks.
- Upstreaming into kata later swaps only the data layer; model and view stay unchanged.
- Tests may run kata's service in process (`kata.New` served by its `Handler()` behind `httptest`) with a temporary database per test. The two-writers problem does not arise for a database only the test owns. The shipped program never does this.
