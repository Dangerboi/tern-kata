# Agents on other hosts are found through Agent records, and messages are trusted by verified sender

A person who runs kata Panes on two machines against one kata daemon cannot reach the agents running on the other machine: every list of agents (the Agent card, the badge, `m`, `S`, the wrap-up, `tern-kata agents`, `tern-kata message`) reads this host's **Agent mapping**. Delivery already works across machines, since the extension follows kata's event stream and a slot written anywhere reaches it. What is missing is finding the agent, and a rule for whom an agent may believe.

This amends [ADR-0014](0014-messages-to-agents-through-tern-katas-omp-extension.md)'s trust rule ("Trust is the sender") and builds on its address and delivery, and on [ADR-0015](0015-event-messages-come-from-the-agents-extension-without-a-slot.md)'s event follower.

## Decisions

- **An Agent record per agent.** Each Pane writes one metadata key on the agent's Issue for every agent it starts, omp or not: `tern-kata.agent.<host id>-<block id>`, holding `{address, host, host_id, block, name, actor, started_at, no_address, message_from}`. A non-omp agent carries its `no_address` reason and an empty `address`; `message_from` is the list from `agent.toml`. The record is static: no state field and no heartbeat, written once at start and removed once at the end. Snapshots already carry Issue metadata and any event makes a Pane re-read its snapshot, so other Panes see a record with no new event code. Agent state (working, idle, waiting) on another host is out of scope.
- **Host id.** A random id made once and kept in `host-id` in tern-kata's state folder goes into every key and value; reconcile and the address check match on it, never on the host name. The **host name** is display only: `host` at the top level of `agent.toml` (not per Project; refused inside `[projects.*]`), default the machine's host name. Attachments keep the machine's host name, which is not the display name.
- **Who writes and removes.** The starting Pane writes the record right after it records the agent in `agents.json`, and removes it wherever it drops the entry: an exit or a closed block (beside the clear of the agent's address), a stop, a Background agent ending, and a reconcile dropping an entry whose block is gone. At Pane start the reconcile also writes a missing record for each mapped agent that has not exited (agents running when this shipped) and removes any record carrying this host id that has no entry in `agents.json`. It never removes a record with another host id. A host that never comes back leaves its records until someone forgets them.
- **Address collision.** Before writing, the Pane looks for a record of another host id on the Issue with the same address. If it finds one it publishes nothing: the agent stays local-only and the entry in `agents.json` keeps the other host's name for the Agent card to show. A later reconcile publishes it when the other record is gone.
- **Verified senders.** Without kata's identity mode (`[auth] require_token_identity = true`) a slot's `from` is whatever the writer claims. A message to an agent on another host, or from an actor other than the agent's own, needs a verified daemon: the sender reads `/api/v1/instance` and treats `auth.kind` `db_token`, `bootstrap` and `trusted_proxy` as verified. An agent takes messages from its own actor and from the actors in its `message_from`. The `kata` CLI exposes no auth kind, so the wrapper reads `/api/v1/instance` at the agent's start and sets `TERN_KATA_VERIFIED_SENDERS=1`; the extension delivers a `message_from` actor's slot only with it set. A daemon switched to identity mode while an agent runs counts for agents started after.
- **Event actor.** `notify.*` is not a reserved metadata key, so `kata meta set <ref> notify.<key> '{"from":"you",…}'` forges a sender even in identity mode. For every slot that arrives as a live `issue.metadata_updated` event the extension therefore delivers only when the event's actor equals the slot's `from` (kata's `notify` handler sets `from` to the attributed actor, so real messages pass). A slot read with `kata inbox` at start or after a gap the stream cannot resume has no event actor, and `kata` has no per-Issue event history to look it up; from that read only the agent's own actor's slots are delivered, as before. That window is the remaining forgery path and is accepted.
- **Teams stay local.** A team starts and drives only agents on its own host.

## Considered and rejected

- **A heartbeat or state in the record:** it turns a static record into a write per change from every agent, and costs every Pane an event per heartbeat. A dead host is handled by Forget instead.
- **Host name as the identity:** two machines can share a name, and a rename would orphan records. The host id is the key; the name is a label.
- **A shared `agents.json`:** it would need a server both machines reach; kata already is one.
- **Issue-scoped tokens per agent** (kata v0.22 `kata tokens create --issue`): a stronger fix for the forgery window, out of scope here.

## Consequences

- Records on an Issue are visible to anyone who can read it, including each host's name and agent addresses.
- A record whose Pane died stays until a Pane of the same host id reconciles it or a person forgets it from another Pane.
- Existing single-host use changes nothing a person sees except the extra metadata key per agent.
