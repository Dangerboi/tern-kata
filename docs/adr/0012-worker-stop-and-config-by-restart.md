# The worker stops through its control socket, and a config change takes effect by restarting it

A running worker never rereads `worker.toml`. A change reaches it by a restart, and `tern-kata worker stop` is how a restart starts: the user (or the Pane) stops the worker and the plugin's host half, which starts a worker again whenever one exits, brings up a fresh one on the new file. This amends [ADR-0010](0010-worker-configuration-shape.md), whose consequence "the worker reads the file once; whether the plugin host restarts it on change belongs to the supervision ticket" is now settled: the host half does not watch the file, and nothing reloads it in place.

## Decisions

- **Stop is a control request** (ADR-0004's socket), with mode `idle` (the default) or `now`. A signal still works and is the same as `now`.
- **Idle drains.** The worker refuses every admission with "the worker is stopping": scheduled occurrences, `run` requests and resumes. It lets the runs in flight finish and report to kata, then exits 0. Held runs wait for a person, not for the worker, so they do not count as in flight; an abandon is still accepted, since it ends a run rather than starting one.
- **Now ends at once.** The worker takes the path a terminate signal takes: the agents are killed and the runs they were driving are held `interrupted` and resumable at the next start (ADR-0006). A `now` while an idle drain waits upgrades it.
- **Leases are kept.** A stopped worker does not release its Projects' leases (ADR-0003); its next start renews them, and another machine takes over only by the lease's own expiry or `--takeover`.
- **The answer says what happens next:** the mode, the runs in flight and whether the plugin will start a fresh worker (the worker was started by the plugin, `TERN_KATA_SUPERVISOR` set, and no `worker.manual` lies beside `worker.toml`). A worker the user started by hand is not started again.
- **Why not reload in place:** the config pins identities (ADR-0010's pin), capacities and checkouts that running runs depend on; one restart path is simpler to reason about than a reload that must decide which running run keeps which old value. The cost, losing in-flight runs, is what the idle mode avoids.
