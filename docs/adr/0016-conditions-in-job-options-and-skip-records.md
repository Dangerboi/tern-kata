# Conditions live in a job's options, and a skip is a worker skip record that settles its occurrence

A due adapter-job run can be made to start only when conditions in the job's definition say yes. The first conditions are new commits and limits. A run that fails a condition is skipped: the worker writes no kata Run for it and records the skip in its own state.

This builds on [ADR-0011](0011-scheduling-triggers-and-catch-up.md) (the checkpoint, catch-up, refresh before admission), [ADR-0007](0007-capacity-and-overlap.md) (slots and the Overlap gate) and [ADR-0010](0010-worker-configuration-shape.md) (what goes in `worker.toml`). The idea comes from tern-agent-scheduler's Only when page.

## Decisions

- **Conditions are stored in the job's `options`.** `options` is the adapter's opaque object that `cronjob.Job` keeps whole. A condition there is shared with every client of the definition, shows in the Job card, and is ignored by a worker that does not know it. Kata stores it; `worker.toml` holds none. Conditions that kata refuses (a local path, such as a later script check) would sit in `worker.toml`; none of the first conditions is one.
- **Behaviour depends on which worker holds the lease.** Another machine's worker that does not know the conditions runs the job anyway. This is accepted: ADR-0003 already makes the lease holder the only worker that admits a Project's runs, and the same is true of any option the worker reads.
- **A skip is not a kata Run.** Kata's Run statuses have no `skipped`. A `cancelled` Run with a `reason:` line would leave a Run, and for a per-run-Issue job an Issue, for every pointless occurrence, which is what the conditions exist to avoid. Run journals are per Run (ADR-0006: `runs/<run uid>/run.json`) and a skip has no Run, so the worker records the skip, with the condition and its reason, in new worker state: a per-Project skip record file in the worker's state folder. This adds to ADR-0006's state, as the failure count did. The Runs Page shows skips as one row from the worker's status. A request to kata for a `skipped` status is filed separately; nothing here waits for it.
- **A skip settles its occurrence.** It leaves the checkpoint's pending list as an admitted occurrence does, so catch-up never retries it. It writes no run journal that closes the Overlap gate, so the gate is untouched and a later occurrence is judged on its own.
- **A condition that cannot be checked blocks the run.** The occurrence stays pending and the user is told once, the way a refusal is logged once today.
- **Conditions are checked at due time, in admission,** after the refresh of the definition (ADR-0011) and before a slot is reserved. They are separate from reactions to kata events as they happen, which are a different mechanism.
- **Conditions are checked only for an occurrence that has no run journal yet.** After a crash between the run journal and the checkpoint save, the occurrence is still pending and is admitted again; the journal that already covers it answers (ADR-0011, ADR-0006). That occurrence is admitted as today and is never skipped, so a skip never contradicts a run that is already journalled.
- **New commits are counted from the last succeeded run.** The job's saved commit is the one its last succeeded run started at, manual runs included (they are not checked, but a succeeded one moves it). A failed, cancelled or held run does not use up commits, so a failed review does not swallow the commits it was meant to review. The job's checkout is the definition's `checkout_key`.

## Considered and rejected

- **A `cancelled` Run with a `reason:` line:** every skip becomes a Run, and a per-run-Issue job would still leave an Issue.
- **Conditions in `worker.toml`:** one job split across two places, and invisible to the Job card and to other workers.

## Consequences

- A skipped occurrence is gone: the job runs again only at its next occurrence, or on Run now.
- Skip records are local to the machine that held the lease; another machine's Runs Page does not show them.
- Spend limits need per-job cost totals across runs, which the worker does not keep today (its Outcome holds tokens only): new worker state.
