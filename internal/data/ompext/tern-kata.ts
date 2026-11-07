// tern-kata's omp extension: delivers kata notification slots written for this
// agent (from tern-kata's Attention Page) to the running omp session as user messages,
// tells the agent when an Issue that blocks its own closes and, when the Pane opted in, when a person comments on its Issue,
// and names the session to `kata close` so a close carries a transcript reference.

import { renameSync, rmSync, writeFileSync } from "node:fs";

/** One running `kata` child whose stdout is read line by line. */
export interface Spawned {
  lines: AsyncIterable<string>;
  exited: Promise<number>;
  kill(): void;
}

/** Effects the follower needs; tests replace them with fakes. */
export interface FollowerDeps {
  address: string;
  project: string;
  /** The agent's Issue, qualified (`project#short id`). */
  ref: string;
  /** Deliver a person's comment on the agent's Issue (TERN_KATA_FORWARD_COMMENTS=1); off when absent. */
  forwardComments?: boolean;
  /** Starts a long-running kata command; args exclude "kata". */
  spawn(args: string[]): Spawned;
  /** Runs a one-shot kata command; args exclude "kata". */
  run(args: string[]): Promise<{ code: number; stdout: string }>;
  deliver(text: string): void;
  /** Milliseconds since the epoch; events created before the follower started are not acted on. */
  now(): number;
  setTimer(fn: () => void, ms: number): unknown;
  clearTimer(handle: unknown): void;
}

/** The follower's lifecycle. */
export interface Follower {
  start(): void;
  stop(): void;
}

/** The value of a notification slot. */
export interface Slot {
  from: string;
  message: string;
  teammate?: string;
  re?: string;
  kind?: string;
}

/** What a stream line means to the follower. */
export type StreamEvent =
  | { type: "reset"; after: number }
  | { type: "slot"; ref: string; eventId: number; to: Slot | null };

/** The agent's Issue and address read from the environment. */
export interface Config {
  address: string;
  project: string;
  ref: string;
  forwardComments?: boolean;
}

/** A stream event other than a slot write, with the fields reactions read. */
export interface KataEvent {
  eventId: number;
  type: string;
  /** The Issue the event is about, `project#short id`. */
  ref: string;
  /** Milliseconds since the epoch. */
  createdAt: number;
  payload: Record<string, unknown>;
}

/** A blocking link of the agent's Issue: the blocker and whether it was open when read. */
export interface Blocker {
  ref: string;
  open: boolean;
}

/** Delivery state kept for one Issue ref: the last event id and value seen, and the stream count when the stream last set it. */
interface RefState {
  eventId?: number;
  value: string | null;
  streamed: number;
}

const MIN_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 30000;

/** Returns the notification slot metadata key for an address. */
export function slotKey(address: string): string {
  return "notify." + Buffer.from(address, "utf8").toString("base64url");
}

/** Returns the actor part of an address (up to its last "/"), or undefined when it has none. */
export function trustedSender(address: string): string | undefined {
  const i = address.lastIndexOf("/");
  return i > 0 ? address.slice(0, i) : undefined;
}

/** Reads KATA_INBOX_USER and KATA_REF, and TERN_KATA_FORWARD_COMMENTS; undefined means the extension must do nothing. */
export function readConfig(env: Record<string, string | undefined>): Config | undefined {
  const address = (env.KATA_INBOX_USER ?? "").trim();
  const ref = (env.KATA_REF ?? "").trim();
  const hash = ref.indexOf("#");
  const project = hash > 0 ? ref.slice(0, hash).trim() : "";
  if (address === "" || project === "" || trustedSender(address) === undefined) return undefined;
  return { address, project, ref, forwardComments: env.TERN_KATA_FORWARD_COMMENTS === "1" || undefined };
}

function asRecord(v: unknown): Record<string, unknown> | undefined {
  return typeof v === "object" && v !== null && !Array.isArray(v) ? (v as Record<string, unknown>) : undefined;
}

function optionalText(v: unknown): string | undefined {
  if (typeof v === "string" && v !== "") return v;
  if (typeof v === "number") return String(v);
  return undefined;
}

/** Narrows a parsed JSON value to a Slot, or undefined when it is not one. */
export function toSlot(v: unknown): Slot | undefined {
  const r = asRecord(v);
  if (!r || typeof r.from !== "string" || typeof r.message !== "string") return undefined;
  return { from: r.from, message: r.message, teammate: optionalText(r.teammate), re: optionalText(r.re), kind: optionalText(r.kind) };
}

/** Returns the leading `"event_id":<n>` of a stream line, or undefined. */
export function eventIdOf(line: string): number | undefined {
  const m = /^\{"event_id":(\d+),/.exec(line);
  return m ? Number(m[1]) : undefined;
}

/** Interprets one stream line; undefined when it is not for this key. */
export function parseStreamLine(line: string, key: string): StreamEvent | undefined {
  const isReset = line.includes('"reset_required"');
  if (!isReset && !line.includes(key)) return undefined;
  let parsed: Record<string, unknown> | undefined;
  try {
    parsed = asRecord(JSON.parse(line));
  } catch {
    return undefined;
  }
  if (!parsed) return undefined;
  if (parsed.reset_required === true && typeof parsed.reset_after_id === "number") {
    return { type: "reset", after: parsed.reset_after_id };
  }
  if (parsed.type !== "issue.metadata_updated") return undefined;
  const diff = asRecord(asRecord(parsed.payload)?.diff);
  const change = diff ? asRecord(diff[key]) : undefined;
  if (!change || typeof parsed.event_id !== "number") return undefined;
  if (typeof parsed.project_name !== "string" || typeof parsed.issue_short_id !== "string") return undefined;
  const ref = `${parsed.project_name}#${parsed.issue_short_id}`;
  if (change.to === null) return { type: "slot", ref, eventId: parsed.event_id, to: null };
  const to = toSlot(change.to);
  return to ? { type: "slot", ref, eventId: parsed.event_id, to } : undefined;
}

/** Interprets one stream line as an event of one of `types`; undefined for any other line. */
export function parseEventLine(line: string, types: readonly string[]): KataEvent | undefined {
  if (!types.some((t) => line.includes(`"${t}"`))) return undefined;
  let parsed: Record<string, unknown> | undefined;
  try {
    parsed = asRecord(JSON.parse(line));
  } catch {
    return undefined;
  }
  if (!parsed || typeof parsed.type !== "string" || !types.includes(parsed.type)) return undefined;
  if (typeof parsed.event_id !== "number" || typeof parsed.project_name !== "string") return undefined;
  if (typeof parsed.issue_short_id !== "string" || typeof parsed.created_at !== "string") return undefined;
  const createdAt = Date.parse(parsed.created_at);
  if (Number.isNaN(createdAt)) return undefined;
  return {
    eventId: parsed.event_id,
    type: parsed.type,
    ref: `${parsed.project_name}#${parsed.issue_short_id}`,
    createdAt,
    payload: asRecord(parsed.payload) ?? {},
  };
}

/** Reads the blockers of `ref` from `kata show --json` output; undefined on bad JSON. Only blocking links count. */
export function parseBlockers(stdout: string, ref: string): Blocker[] | undefined {
  let links: unknown;
  try {
    links = asRecord(JSON.parse(stdout))?.links;
  } catch {
    return undefined;
  }
  if (!Array.isArray(links)) return undefined;
  const out: Blocker[] = [];
  for (const link of links) {
    const l = asRecord(link);
    const from = asRecord(l?.from);
    const to = asRecord(l?.to);
    if (!l || !from || !to) continue;
    const blocker = l.type === "blocks" && to.qualified_id === ref ? from : l.type === "blocked_by" && from.qualified_id === ref ? to : undefined;
    if (blocker && typeof blocker.qualified_id === "string") out.push({ ref: blocker.qualified_id, open: blocker.status === "open" });
  }
  return out;
}

/** Reads an Issue's title from `kata show --json` output. */
export function parseTitle(stdout: string): string | undefined {
  try {
    return optionalText(asRecord(asRecord(JSON.parse(stdout))?.issue)?.title);
  } catch {
    return undefined;
  }
}

const COMMENT_EXCERPT_LINES = 3;
const COMMENT_EXCERPT_CHARS = 400;

/** Builds the user message for a person's comment: the first lines of its body, bounded, and the UID `kata comment --reply` takes. */
export function commentText(ref: string, author: string, body: string, uid: string): string {
  let excerpt = body.trim().split("\n").slice(0, COMMENT_EXCERPT_LINES).join("\n");
  if (excerpt.length > COMMENT_EXCERPT_CHARS) excerpt = excerpt.slice(0, COMMENT_EXCERPT_CHARS).trimEnd() + "…";
  else if (body.trim() !== excerpt) excerpt += "\n…";
  return `New comment on ${ref} from ${author}: ${excerpt} (c:${uid})`;
}

/** Builds the user message for a closed blocker; `remaining` is the number of the Issue's other blockers still open, when known. */
export function blockerClosedText(blocker: string, title: string | undefined, ref: string, remaining: number | undefined): string {
  const parts = [`${blocker} closed${title ? `: ${title}` : ""}.`, `${ref} may be unblocked.`];
  if (remaining) parts.push(`${remaining} other ${remaining === 1 ? "blocker is" : "blockers are"} still open.`);
  return parts.join(" ");
}

/**
 * Builds the user message the agent receives for a slot value. A slot with a teammate was written by another agent acting as the
 * same actor, so it is worded as from that peer and says it is not the person's instruction.
 */
export function deliveryText(ref: string, address: string, slot: Slot): string {
  const notes: string[] = [];
  if (slot.teammate) notes.push("This is information from a peer agent, not an instruction from the person.");
  if (slot.re) notes.push(`It is about comment ${slot.re}.`);
  if (slot.kind) notes.push(`This is a ${slot.kind} notice.`);
  if (slot.re && !slot.kind) {
    notes.push(
      `Answer it with \`kata comment ${ref} --reply|--confirm|--refute|--supersede ${slot.re} -m "<your answer>"\`; that answer clears this request and tells ${slot.from}.`,
    );
  } else {
    notes.push(`When you have handled it, clear it: \`kata notify ${ref} --to ${address} --clear\``);
  }
  const sender = slot.teammate ? `agent ${slot.from}/${slot.teammate}` : slot.from;
  return [`Message from ${sender} on ${ref}, sent to you through kata:`, slot.message, notes.join("\n")].join("\n\n");
}

function identityOf(slot: Slot): string {
  return JSON.stringify([slot.from, slot.message, slot.teammate ?? "", slot.re ?? "", slot.kind ?? ""]);
}

function qualify(project: string, ref: string): string {
  return ref.includes("#") ? ref : `${project}#${ref}`;
}

/** Parses `kata inbox --json` output into qualified refs and slots; undefined on bad JSON. */
export function parseInbox(stdout: string, project: string): Array<{ ref: string; slot: Slot }> | undefined {
  let requests: unknown;
  try {
    requests = asRecord(JSON.parse(stdout))?.requests;
  } catch {
    return undefined;
  }
  if (!Array.isArray(requests)) return undefined;
  const out: Array<{ ref: string; slot: Slot }> = [];
  for (const item of requests) {
    const slot = toSlot(item);
    const ref = asRecord(item)?.ref;
    if (slot && typeof ref === "string" && ref !== "") out.push({ ref: qualify(project, ref), slot });
  }
  return out;
}

/**
 * Creates the follower: it tails kata's event stream, re-reads the inbox when the
 * stream restarts or resets, and delivers each new trusted slot value once.
 */
export function createFollower(deps: FollowerDeps): Follower {
  const key = slotKey(deps.address);
  const trusted = trustedSender(deps.address);
  const states = new Map<string, RefState>();
  let running = false;
  let generation = 0;
  let child: Spawned | undefined;
  let timer: unknown;
  let cursor = 0;
  let backoff = MIN_BACKOFF_MS;
  let reading = false;
  let readAgain = false;
  // streamed counts the stream's slot events, so a read can leave alone the refs the stream changed while it ran.
  let streamed = 0;
  // Event reactions: what the follower does with a stream event that is not a slot write. A new reaction is one more entry.
  const reactions: Record<string, (event: KataEvent, gen: number) => Promise<void>> = {
    "issue.closed": onClosed,
    "issue.links_changed": onLinksChanged,
    "issue.linked": onLinkEdge,
    "issue.unlinked": onLinkEdge,
    ...(deps.forwardComments ? { "issue.commented": onCommented } : {}),
  };
  const reactionTypes = Object.keys(reactions);
  const shortId = deps.ref.slice(deps.ref.indexOf("#") + 1);
  let startedAt = 0;
  let reacted = 0;
  let blockers = new Set<string>();
  // queue runs reactions one at a time in stream order, the first being the read of the blockers at start.
  let queue: Promise<void> = Promise.resolve();

  function enqueue(gen: number, task: () => Promise<void>): void {
    queue = queue.then(() => (gen === generation ? task() : undefined)).catch(() => {});
  }

  async function showOf(ref: string): Promise<string | undefined> {
    try {
      const result = await deps.run(["show", ref, "--json"]);
      return result.code === 0 ? result.stdout : undefined;
    } catch {
      return undefined;
    }
  }

  async function readBlockers(gen: number): Promise<Blocker[] | undefined> {
    const stdout = await showOf(deps.ref);
    const list = stdout === undefined || gen !== generation ? undefined : parseBlockers(stdout, deps.ref);
    if (list) blockers = new Set(list.map((b) => b.ref));
    return list;
  }

  async function onLinksChanged(event: KataEvent, gen: number): Promise<void> {
    const names = (field: string): boolean => {
      const value = event.payload[field];
      return Array.isArray(value) && value.some((v) => v === shortId || v === deps.ref);
    };
    if (event.ref === deps.ref || names("blocks_added") || names("blocks_removed")) await readBlockers(gen);
  }

  // kata's links endpoint reports one link as issue.linked or issue.unlinked, naming both ends in from_short_id and to_short_id.
  async function onLinkEdge(event: KataEvent, gen: number): Promise<void> {
    const ends = [event.payload.from_short_id, event.payload.to_short_id];
    if (event.ref === deps.ref || ends.some((v) => v === shortId || v === deps.ref)) await readBlockers(gen);
  }

  async function onClosed(event: KataEvent, gen: number): Promise<void> {
    if (!blockers.has(event.ref)) return;
    const [mine, theirs] = await Promise.all([readBlockers(gen), showOf(event.ref)]);
    if (gen !== generation) return;
    const remaining = mine?.filter((b) => b.open && b.ref !== event.ref).length;
    deps.deliver(blockerClosedText(event.ref, theirs === undefined ? undefined : parseTitle(theirs), deps.ref, remaining));
  }

  // A comment with a teammate is an agent's (every Pane-started agent writes with KATA_TEAMMATE), so only a person's is delivered.
  async function onCommented(event: KataEvent): Promise<void> {
    if (event.ref !== deps.ref || optionalText(event.payload.teammate) !== undefined) return;
    const uid = optionalText(event.payload.comment_uid);
    const body = event.payload.body;
    if (uid === undefined || typeof body !== "string" || body.trim() === "") return;
    deps.deliver(commentText(deps.ref, optionalText(event.payload.author) ?? "someone", body, uid));
  }

  // react acts on an event created after the follower started, once per event id.
  function react(event: KataEvent): void {
    if (event.createdAt <= startedAt || event.eventId <= reacted) return;
    reacted = event.eventId;
    const gen = generation;
    enqueue(gen, () => reactions[event.type](event, gen));
  }

  function handle(ref: string, eventId: number | undefined, slot: Slot | null): void {
    const stored = states.get(ref);
    if (eventId !== undefined && stored?.eventId !== undefined && eventId <= stored.eventId) return;
    const seen = eventId === undefined ? (stored?.streamed ?? 0) : ++streamed;
    const identity = slot === null ? null : identityOf(slot);
    if (stored && stored.value === identity) {
      if (eventId !== undefined) stored.eventId = eventId;
      stored.streamed = seen;
      return;
    }
    states.set(ref, { eventId: eventId ?? stored?.eventId, value: identity, streamed: seen });
    if (slot === null || slot.from !== trusted || slot.message.trim() === "") return;
    deps.deliver(deliveryText(ref, deps.address, slot));
  }

  // applyInbox takes a read that began when streamed was since: a ref the stream changed after that holds newer news than the read.
  function applyInbox(entries: Array<{ ref: string; slot: Slot }>, since: number): void {
    const newer = (ref: string) => (states.get(ref)?.streamed ?? 0) > since;
    const listed = new Set<string>();
    for (const { ref, slot } of entries) {
      listed.add(ref);
      if (!newer(ref)) handle(ref, undefined, slot);
    }
    for (const [ref, state] of states) {
      if (state.value !== null && !listed.has(ref) && !newer(ref)) state.value = null;
    }
  }

  async function readInboxOnce(gen: number): Promise<void> {
    const since = streamed;
    let result: { code: number; stdout: string };
    try {
      result = await deps.run(["inbox", "--for", deps.address, "--json", "--project", deps.project]);
    } catch {
      return;
    }
    if (gen !== generation || result.code !== 0) return;
    const entries = parseInbox(result.stdout, deps.project);
    if (entries) applyInbox(entries, since);
  }

  async function readInbox(): Promise<void> {
    if (reading) {
      readAgain = true;
      return;
    }
    reading = true;
    try {
      do {
        readAgain = false;
        await readInboxOnce(generation);
      } while (readAgain && running);
    } finally {
      reading = false;
    }
  }

  function onLine(line: string): void {
    const id = eventIdOf(line);
    if (id !== undefined && id > cursor) cursor = id;
    const event = parseStreamLine(line, key);
    if (!event) {
      const other = parseEventLine(line, reactionTypes);
      if (other) react(other);
      return;
    }
    if (event.type === "reset") {
      cursor = event.after;
      void readInbox();
      return;
    }
    handle(event.ref, event.eventId, event.to);
  }

  function spawnStream(): Spawned | undefined {
    const args = ["events", "--tail", "--json", "--project", deps.project];
    if (cursor > 0) args.push("--last-event-id", String(cursor));
    try {
      return deps.spawn(args);
    } catch {
      return undefined;
    }
  }

  async function follow(gen: number, stream: Spawned): Promise<void> {
    let sawLine = false;
    try {
      for await (const line of stream.lines) {
        if (gen !== generation) return;
        sawLine = true;
        onLine(line);
      }
      await stream.exited;
    } catch {
      // a broken stream is handled like an exit
    }
    if (gen !== generation) return;
    scheduleRestart(gen, sawLine);
  }

  function scheduleRestart(gen: number, sawLine: boolean): void {
    if (sawLine) backoff = MIN_BACKOFF_MS;
    const delay = backoff;
    backoff = Math.min(backoff * 2, MAX_BACKOFF_MS);
    timer = deps.setTimer(() => {
      if (gen !== generation) return;
      timer = undefined;
      launch(gen);
      void readInbox();
    }, delay);
  }

  function launch(gen: number): void {
    child = spawnStream();
    if (child) void follow(gen, child);
    else scheduleRestart(gen, false);
  }

  return {
    start() {
      if (running) return;
      running = true;
      generation++;
      backoff = MIN_BACKOFF_MS;
      startedAt = deps.now();
      const gen = generation;
      enqueue(gen, async () => void (await readBlockers(gen)));
      launch(generation);
      void readInbox();
    },
    stop() {
      if (!running) return;
      running = false;
      generation++;
      if (timer !== undefined) deps.clearTimer(timer);
      timer = undefined;
      try {
        child?.kill();
      } catch {
        // already gone
      }
      child = undefined;
    },
  };
}

/** The slice of omp's extension context this file uses. */
interface ExtensionContext {
  cwd?: string;
  agent?: { kind?: string };
  sessionManager?: { getSessionId?(): string; getSessionFile?(): string | undefined };
  setTimeout?(fn: () => void, ms: number): unknown;
  clearTimer?(handle: unknown): void;
}

/** The omp events after which the session's id may be another one; omp re-reads process.env for its shells after them. */
const SESSION_EVENTS = ["session_start", "session_switch", "session_branch"] as const;

/** The slice of omp's extension API this file uses. */
export interface ExtensionApi {
  on(event: (typeof SESSION_EVENTS)[number] | "agent_start" | "session_shutdown", handler: (event: unknown, ctx: ExtensionContext) => void): void;
  sendUserMessage(text: string, options: { deliverAs: "aside" }): Promise<unknown> | void;
}

/** kata's rule for the session id of an agent other than claude and codex (its internal/transcript). */
const TRANSCRIPT_ID = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;

/**
 * Names the main session to `kata close`, which reads KATA_TRANSCRIPT_AGENT and KATA_TRANSCRIPT_SESSION_ID when
 * [close.transcript] is enabled in kata's config. A session id kata would refuse unsets both, so a close never cites
 * the session before a switch.
 */
export function exportTranscriptSession(env: Record<string, string | undefined>, ctx: ExtensionContext): void {
  let id = "";
  try {
    id = ctx.sessionManager?.getSessionId?.() ?? "";
  } catch {
    // no session id to name
  }
  if (TRANSCRIPT_ID.test(id)) {
    env.KATA_TRANSCRIPT_AGENT = "omp";
    env.KATA_TRANSCRIPT_SESSION_ID = id;
  } else {
    delete env.KATA_TRANSCRIPT_AGENT;
    delete env.KATA_TRANSCRIPT_SESSION_ID;
  }
}

/** The suffix of the file beside an agent's prompt file that holds its session file's path; the Pane reads it (data.AgentSessionSuffix). */
export const SESSION_PATH_SUFFIX = ".session";

/** The prompt file omp was started with: the last `@file` argument whose name is tern-kata's (`prompt-*.md`), or undefined. */
export function promptFileFromArgv(argv: readonly string[]): string | undefined {
  for (let i = argv.length - 1; i >= 0; i--) {
    const arg = argv[i];
    if (arg.startsWith("@") && /(^|[\\/])prompt-[^\\/]*\.md$/.test(arg)) return arg.slice(1);
  }
  return undefined;
}

/**
 * Writes the main session's file path beside the agent's prompt file, so the Pane can read the session without
 * knowing omp's session folder. The write is atomic (a temporary file renamed over the old one); a session with no
 * file yet, a path equal to `written` (the one the last call wrote), or a write that fails, leaves the previous file
 * alone. Returns the path the file holds after a write, else undefined.
 */
export function writeSessionPath(promptFile: string | undefined, ctx: ExtensionContext, written?: string): string | undefined {
  if (!promptFile) return undefined;
  let session: string | undefined;
  try {
    session = ctx.sessionManager?.getSessionFile?.();
  } catch {
    return undefined;
  }
  if (!session || session === written) return undefined;
  const target = promptFile + SESSION_PATH_SUFFIX;
  const temp = `${target}.${process.pid}.tmp`;
  try {
    writeFileSync(temp, session + "\n");
    renameSync(temp, target);
    return session;
  } catch {
    try {
      rmSync(temp, { force: true });
    } catch {
      // the Pane shows the agent without session facts
    }
    return undefined;
  }
}

/** How kata commands are executed; replaced in tests. */
export interface KataIo {
  spawn(args: string[], cwd: string | undefined): Spawned;
  run(args: string[], cwd: string | undefined): Promise<{ code: number; stdout: string }>;
}

async function* readLines(stream: ReadableStream<Uint8Array>): AsyncGenerator<string> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffered = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffered += decoder.decode(value, { stream: true });
    let nl = buffered.indexOf("\n");
    while (nl >= 0) {
      const line = buffered.slice(0, nl);
      buffered = buffered.slice(nl + 1);
      if (line !== "") yield line;
      nl = buffered.indexOf("\n");
    }
  }
  if (buffered !== "") yield buffered;
}

/** Runs kata through Bun with the process environment unchanged. */
export const bunKataIo: KataIo = {
  spawn(args, cwd) {
    const proc = Bun.spawn(["kata", ...args], { cwd, stdout: "pipe", stderr: "ignore", env: process.env });
    return { lines: readLines(proc.stdout), exited: proc.exited, kill: () => proc.kill() };
  },
  async run(args, cwd) {
    const proc = Bun.spawn(["kata", ...args], { cwd, stdout: "pipe", stderr: "ignore", env: process.env });
    const [stdout, code] = await Promise.all([new Response(proc.stdout).text(), proc.exited]);
    return { code, stdout };
  },
};

/** Builds the omp extension factory for an environment and kata runner. */
export function createExtension(
  env: Record<string, string | undefined>,
  io: KataIo,
  argv: readonly string[] = process.argv,
): (pi: ExtensionApi) => void {
  return (pi) => {
    const config = readConfig(env);
    const promptFile = promptFileFromArgv(argv);
    let ctx: ExtensionContext = {};
    let follower: Follower | undefined;

    function build(c: Config): Follower {
      return createFollower({
        ...c,
        spawn: (args) => io.spawn(args, ctx.cwd),
        run: (args) => io.run(args, ctx.cwd),
        now: () => Date.now(),
        deliver: (text) => {
          try {
            Promise.resolve(pi.sendUserMessage(text, { deliverAs: "aside" })).catch(() => {});
          } catch {
            // delivery is best effort
          }
        },
        setTimer: (fn, ms) => {
          const guarded = () => {
            try {
              fn();
            } catch {
              // a timer must never throw into omp
            }
          };
          return ctx.setTimeout ? ctx.setTimeout(guarded, ms) : setTimeout(guarded, ms);
        },
        clearTimer: (handle) => {
          if (ctx.clearTimer) ctx.clearTimer(handle);
          else clearTimeout(handle as never);
        },
      });
    }

    // omp's `/move` renames the session file without a session event, so each turn's start re-reads the path too.
    let writtenSession: string | undefined;
    const writeSession = (c: ExtensionContext) => {
      writtenSession = writeSessionPath(promptFile, c, writtenSession) ?? writtenSession;
    };
    for (const name of SESSION_EVENTS) {
      pi.on(name, (_event, c) => {
        // An in-process subagent shares process.env with its parent, whose commands must keep naming the parent.
        if (c.agent?.kind === "sub") return;
        exportTranscriptSession(env, c);
        writeSession(c);
        if (name !== "session_start" || !config) return;
        ctx = c;
        follower ??= build(config);
        follower.start();
      });
    }
    pi.on("agent_start", (_event, c) => {
      if (c.agent?.kind === "sub") return;
      writeSession(c);
    });
    pi.on("session_shutdown", () => {
      follower?.stop();
    });
  };
}

export default function ternKata(pi: ExtensionApi): void {
  createExtension(process.env, bunKataIo)(pi);
}
