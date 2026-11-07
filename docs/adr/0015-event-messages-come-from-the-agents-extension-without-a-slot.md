# Event messages reach an agent from its own extension, with no slot

Some messages to an omp agent are not written by anyone: they follow from something that happens in kata, such as an Issue that blocks the agent's Issue closing. tern-kata's omp extension delivers these itself, from the events it already follows, and writes no notification slot for them.

This builds on [ADR-0014](0014-messages-to-agents-through-tern-katas-omp-extension.md), whose extension already tails `kata events --tail --json --project <project>` with full payloads and knows its Issue from `KATA_REF`. It departs from ADR-0014's path for a person's message (the Pane writes a slot, the extension delivers the slot's value) for messages that have no sender, and it keeps ADR-0014's delivery call, `pi.sendUserMessage(text, { deliverAs: "aside" })`, the same `deliver` the slot path uses.

## Decisions

- **The extension reacts to events other than its slot.** One table in the follower maps an event type to its reaction, so a further reaction is one more entry beside the first. The first is a blocker's close: when an `issue.closed` names an Issue that blocks the agent's Issue, the agent is told "<blocker ref> closed: <title>. <ref> may be unblocked.", with how many of its other blockers are still open.
- **Blockers are read, not guessed.** The extension reads its Issue's blocking links with `kata show <ref> --json` at start and again on an `issue.links_changed` that names its Issue, or names it in `blocks_added` or `blocks_removed` of another. When a blocker closes it reads what the close event lacks, the blocker's title and the blockers still open, with the same command. Parent and related links are never read as blockers.
- **Only events after the start are acted on.** A stream opened without a cursor replays up to 10 000 old events, so an event whose `created_at` is not after the follower's start is ignored. After the stream restarts with `--last-event-id`, the events since the cursor are acted on, each once by its event id. A blocker reopened and closed again is a new event and tells the agent again.
- **Reactions run one at a time, in stream order,** the read of the blockers at start being the first, so a close that arrives while that read is out waits for it.
- **A failed read costs a part of the message, not the message.** Without the blocker's title the text names the ref alone; without the Issue's blockers it omits the count.

## Considered and rejected

- **The Pane writes a slot per event:** the message would exist only while a Pane runs on the machine that started the agent, and a slot is one value per Issue and address, so two closes in a row would replace each other. The slot also asks the agent to clear it, which an event with no sender has no use for.
- **Reading the agent's Issue links on every close:** one `kata show` for every close in the Project; the blockers are read at start and when links change instead, and only a close of a known blocker reads more.

## Consequences

- Delivery works with the Pane closed and for an agent on another machine than any Pane, since the agent's own extension sees the stream.
- Only blockers in the agent's own Project are seen: the stream is per Project, so a blocker in another Project sends nothing.
- A close that happened while the agent was not running, or before the extension started, is never delivered; an agent that starts on an Issue whose blockers are already closed reads that from the Issue.
- Each reaction costs a line check per stream line, a JSON parse only for the event types the table names.
- The extension is versioned with tern-kata, as in ADR-0014: a running agent keeps the one it loaded.
