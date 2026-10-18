# Talk to the kata daemon; never embed kata

tern-kata reads and writes kata only through the running daemon's HTTP API, using the typed client `go.kenn.io/kata/pkg/client`. It does not embed `go.kenn.io/kata`, even though that module is importable and would remove the daemon dependency.

Embedding constructs a `kata.Service` that owns its own storage handle. Running one beside the daemon that the CLI, agents and `kata tui` already use would mean two writers on one database, or a second database nobody else sees. kata's architecture treats the daemon's HTTP service as the single access path, so every client sees the same validation, actor attribution, event ordering and hooks.

## Consequences

- A daemon must be running; tern-kata never starts one. We find it with kata's own `client.Discover` for the workspace, which picks the daemon and its credentials and handles Unix sockets. With no reachable daemon the Pane shows kata's message plus a hint to start the daemon or run `kata doctor`. Whether the daemon is remote still comes from `kata daemon locate --json`, until kata's client exposes it.
- Live updates come from the daemon's durable event stream, not in-process callbacks.
- Upstreaming into kata later swaps only the data layer; model and view stay unchanged.
- Tests may run kata's service in process (`kata.New` served by its `Handler()` behind `httptest`) with a temporary database per test. The two-writers problem does not arise for a database only the test owns. The shipped program never does this.
