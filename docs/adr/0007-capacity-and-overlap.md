# Slots are reserved before preparation and held runs keep them until their process is gone

Each served Project has a number of run slots (default 1). A slot is reserved before a run is prepared and stays reserved while the run is prepared, admitted, running, or held with a process that may still be alive. A job's overlap gate is separate and stays closed until its earlier run settles or is abandoned.

herdr-kata (state.md "Slots and overlap", run-lifecycle.md "Held capacity") keeps both and fixed bugs in both; the rules carry over, with the proof changed from "pane and agent gone" to "process group gone".

## Decisions

- **Reservation: adopt.** A reservation is made before preparation and rebuilt from journals at start, so a lowered limit never revokes work a previous worker reserved. A durable prepared journal keeps its slot while admission or reporting waits. Waiting for readiness (an existing Issue not yet open) or a Project's lease takes no slot.
- **Held capacity: adopt.** A held run keeps its slot while any recorded process group may be alive. Held runs are rechecked every 10 to 12 minutes and whenever admission finds the Project full. When every group is proven gone the slot is released; the held outcome and the overlap gate remain, and nothing is sent. Pre-admission holds release both at once. A resumed run takes a new reservation first.
- **Overlap: adopt.** `forbid`: pending occurrences wait until the earlier run is settled or abandoned, even if its slot was released. `allow`: admit as slots free. The gate key includes the Project uid. For `forbid` the gate check and the creation of the journal that closes it are one step per Project and job, so two requests arriving together admit one even with free slots. Resume and abandon take one owner per run, so two resumes of one hold never touch the slot the first took.
- **Release: adapt.** herdr-kata's `release` frees slot and gate even if the agent keeps running. `A` (ADR-0005) frees both only after the groups are proven gone; the worker never abandons a live process into free capacity.
- **Limits: adapt.** Per-Project `capacity` defaults to 1 (spec; herdr-kata defaults to 2 globally). A worker-wide `max_active_runs`, unset by default, caps all Projects together. Both apply and held runs count against both. A job's `timeout_seconds` overrides `default_timeout` (2 h), capped by `max_timeout` (8 h); a job over the cap is unsupported with a reason naming both values.
- **Queue order: not specified by herdr-kata; ours.** Due occurrences of one Project admit oldest first.
- **Bound on queued work: adopt.** Catch-up keeps at most 7 days and 100 pending occurrences per job; the oldest overflow is dropped and the decision logged (ADR-0011).
- **Who decides: the Run lifecycle.** Which runs occupy a slot and which close their job's gate are the Run lifecycle's answers (`internal/worker/runlife.go`: `occupiesSlot`, `closesGate`), asked from the journal and the group proof; slot rebuilding at start, the retry pass, the release after a run and the overlap check at request time all ask it. A hold from before admission frees both at once; the secret, prompt and command holds free the slot and keep the gate; the launch-unproven, interrupted, no-receipt and group-unproven holds keep the slot while the group is not proven gone, and with no start record that cannot be proven.

## Consequences

- A held run on a job with `overlap: forbid` blocks that job until the user resumes or abandons it. The Jobs page must say so; the Runs page shows the gate.
- One stuck process can consume a Project's only slot. A timeout bounds this for running runs; a held run needs the user.
