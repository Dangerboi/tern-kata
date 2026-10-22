# The worker follows one event stream per Project and reconciles on a slow, jittered timer

The worker keeps one Server-Sent Events stream per served Project, uses it to learn that definitions or Issues changed, and re-reads from kata on a slow timer in case it missed something. The stream cursor is a hint, never evidence that work was admitted or done.

herdr-kata (kata-adapter.md "Transport and events", architecture.md) arrived at this after polling too often. Its intervals carry over; its process plumbing does not.

## Decisions

- **Stream: adapt.** herdr-kata forks `kata events --tail` per Project, with a macOS wrapper and watchdog so a killed worker's tail process ends. The worker instead calls `StreamEvents` through the same client package as the Pane (`internal/data/events.go` is the prior art) with `project_id` set and `after_id` from the saved cursor. Closing the request ends the stream with the process, so there is no child to leak. kata sends a `: keepalive` comment every 25 s and `sync.reset_required` when a cursor is stale or access changed; a reset triggers a full refresh and a reopen from the current position.
- **What wakes what: adopt.** Only `cron.job.*` and `cron.workflow.*` mark definitions dirty. Issue events wake readiness checks for existing-Issue runs and Issue-date triggers (ADR-0011). Run-observation events wake nothing.
- **Refresh: adapt.** herdr-kata holds the start refresh until the stream's first line plus 250 ms of quiet, capped at 2 s or 1000 lines, because kata replays every `cron.job.*` event to a stream started from event 0. Here any dirty mark schedules one definition refresh (jobs and workflows lists) after 250 ms without further events, at most once every 2 s. A replay of N events is one refresh, and the rule has no start-up special case. Dirty flags survive failed refreshes; the cursor advances after an event is handled or marked.
- **Reconnect: adopt.** A dropped stream reconnects after 0.5 to 1 s, doubling to a 5 minute cap; one that delivered an event or stayed open 5 minutes resets the backoff.
- **Fallback wakes: adopt.** Definitions refresh every 15 to 18 minutes; readiness and held-capacity checks every 10 to 12 minutes. Each Project picks its own jittered delay so Projects started together do not poll together. A Project with no adapter jobs waits 2.5 to 5 minutes instead and returns to the normal cadence at its first job.
- **Timers: adapt.** herdr-kata sleeps to the next occurrence. The worker also wakes at least every minute and recomputes what is due, so a sleeping laptop or a clock change is noticed. The Pane's day-end wake already does the same.
- **Route and access checks: adapt.** herdr-kata probes `cron/capabilities` every 30 to 45 s. A dropped stream is already the signal for a lost route, so the worker checks capabilities on every stream (re)connect, before each admission (a passing check is reused for 5 s), and every 5 minutes while idle. The check requires `cron_v1` and the pinned Project and instance uids (ADR-0010). Failure stops admission and report delivery for that Project until a check passes; the backoff starts at 30 s, doubles to 5 minutes, and is jittered between half and all of its limit.
- **Call limits: adopt, simplified.** At most four one-shot kata calls at once across Projects; event streams take no slot. Each call has a 60 s deadline from when it takes a slot. Read-only checks retry deadline failures up to three times at 1 s then 2 s. Mutations are never retried by the transport; their journal intents decide (ADR-0006). Two Projects start at a time.
- **Control polling and local status files: reject.** herdr-kata's worker reads request files on a 5 s tick and the TUI reads status snapshots so it never polls kata. Requests reach the worker over its control socket, which wakes it at once, and the Pane reads `worker status` over the same socket plus kata directly through the Daemon gateway.
- **Inbox scanning: reject.** There is no native-child pickup (ADR-0002), so no scan, debounce or acknowledgement.
- **Kata CLI as transport: reject.** herdr-kata drives the `kata` binary. ADR-0001 already requires the HTTP client.

## Consequences

- The worker needs kata 0.19.0 or later; `worker doctor` says so and names the missing capability.
- An hour of lost events is repaired by the next reconciliation, never by replaying the stream.
