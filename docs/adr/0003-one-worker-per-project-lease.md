# One worker per Project, held by a lease in the Project's metadata

Across machines, a worker admits runs for a Project only while it holds that Project's lease. The lease is the `tern_kata.lease` key of the Project's own metadata, written with a conditional write against the Project's revision. Locally, one worker per machine takes a lock file.

herdr-kata's lane lease (kata-adapter.md "Lane ownership") keeps a lease in the metadata of a closed Issue labelled `herdr-kata:lease`. We keep its protocol and move it to a place kata gives us for the purpose.

## Decisions

- **Storage: adapt.** kata's `POST /api/v1/projects/{id}/metadata` takes a patch and an `If-Match: "rev-N"` header and compares N with the Project's revision inside the transaction. Observed on a throwaway daemon: a write at `rev-1` succeeded and returned `"rev-2"`; a second write at `rev-1` got 412 `revision_conflict`; unknown keys are accepted. So the lease needs no Issue: no created-then-closed record in every shared Project, no "if two installations create one at once, both use the oldest" rule, no label to hide. Value: `{version: 1, owner: <executor label>, generation, heartbeat}`.
- **Claim and renewal: adopt.** The Project's lease is read once, before the worker lists definitions, opens its stream or admits anything. No lease, an expired one, or its own: claim it (an expired one takes the next generation). The owner renews every 3 minutes and immediately before every admission. A lease unrenewed for 10 minutes has expired.
- **Refusal: adopt.** Another executor's live lease: admit nothing, show `another worker (<executor>) owns this Project` in `worker status`, read again after 3 to 5 minutes, then at most every 5 minutes with jitter. An unreadable lease is unknown, not a refusal: admit nothing and retry with a doubling limit from 10 s to 5 min, each wait random between half and all of the limit.
- **Generation per run: adopt.** A run records the generation it was prepared under. Its Issue create and admission observation go out only while the Project still holds that generation; otherwise the run is held `admission_rejected` before kata sees it. An admitted run belongs to the executor that admitted it.
- **Takeover: adopt.** `tern-kata worker --takeover` writes the next generation. The former owner sees it at its next renewal, admission or event, stops admitting, keeps supervising and settling the runs it already admitted, and holds any it had prepared but not admitted. A deliberate move needs this; a dead machine needs only the 10 minutes.
- **Retire: adopt.** A Project removed from the config has its lease owner cleared at the next start (only this executor's, keeping the generation) and its saved reports delivered, once the route check a served Project gets (ADR-0004, with the pinned daemon instance, ADR-0010) passes; until then nothing is delivered and the lease is left, and the retire is tried again on its backoff. A clean stop does not release: the plugin supervisor restarts the worker and reclaims its own lease at once.
- **Refused-lane behaviour: adopt.** While refused, `run`, `resume` and `abandon` for that Project fail at once with the owner named.
- **Lease issue, `up --takeover` supervisor handshake: reject.** The Issue form is replaced as above. There is no separate supervisor process to stop (ADR-0006).

## Consequences

- Each renewal is a `project.metadata_updated` event and a Project revision bump, every 3 minutes per served Project. Panes refresh on any event, so they re-read a Collection snapshot each time; the snapshot request is conditional. Nothing else reads the Project revision.
- Any writer of the Project's metadata between our read and write makes the conditional write fail with 412. The worker re-reads and retries; if the lease is still its own the retry succeeds.
- Executor labels must be unique; two workers sharing one defeat the lease, as in herdr-kata. The config requires the label and never defaults it to something shared (ADR-0010).
- The lease is advisory outside kata: a worker that cannot write Project metadata (read-only token, spoke replica) cannot serve the Project and says so.
