import { describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, readdirSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  createExtension,
  createFollower,
  promptFileFromArgv,
  readConfig,
  slotKey,
  type ExtensionApi,
  type FollowerDeps,
  type KataIo,
  type Spawned,
} from "./tern-kata";

const ADDRESS = "dangerboi/omp-123";
const KEY = slotKey(ADDRESS);
const REF = "demo#3909";

async function flush(): Promise<void> {
  for (let i = 0; i < 50; i++) await Promise.resolve();
}

interface FakeTimer {
  fn: () => void;
  ms: number;
  cleared: boolean;
}

interface FakeChild extends Spawned {
  args: string[];
  push(line: string): void;
  exit(code?: number): void;
  killed: boolean;
}

function fakeChild(args: string[]): FakeChild {
  const queue: string[] = [];
  let wake: (() => void) | undefined;
  let ended = false;
  const { promise: exited, resolve: resolveExit } = Promise.withResolvers<number>();
  async function* lines(): AsyncGenerator<string> {
    for (;;) {
      if (queue.length > 0) yield queue.shift() as string;
      else if (ended) return;
      else {
        const { promise, resolve } = Promise.withResolvers<void>();
        wake = resolve;
        await promise;
      }
    }
  }
  const child: FakeChild = {
    args,
    killed: false,
    lines: lines(),
    exited,
    push(line) {
      queue.push(line);
      wake?.();
    },
    exit(code = 1) {
      ended = true;
      wake?.();
      resolveExit(code);
    },
    kill() {
      child.killed = true;
      child.exit(0);
    },
  };
  return child;
}

const NOW = Date.parse("2026-11-07T10:00:00Z");

function harness(inboxStdout: () => string = () => emptyInbox(), options: Partial<FollowerDeps> = {}) {
  const children: FakeChild[] = [];
  const runs: string[][] = [];
  const delivered: string[] = [];
  const timers: FakeTimer[] = [];
  const shows = new Map<string, string>();
  let inbox = inboxStdout;
  let held: Array<(stdout: string) => void> | undefined;
  const deps: FollowerDeps = {
    address: ADDRESS,
    project: "demo",
    ref: REF,
    ...options,
    now: () => NOW,
    spawn(args) {
      const c = fakeChild(args);
      children.push(c);
      return c;
    },
    async run(args) {
      if (args[0] === "show") return { code: 0, stdout: shows.get(args[1]) ?? showJson(args[1], "", []) };
      runs.push(args);
      const waiting = held;
      if (waiting) {
        const { promise, resolve } = Promise.withResolvers<string>();
        waiting.push(resolve);
        return { code: 0, stdout: await promise };
      }
      return { code: 0, stdout: inbox() };
    },
    deliver: (text) => delivered.push(text),
    setTimer(fn, ms) {
      const t: FakeTimer = { fn, ms, cleared: false };
      timers.push(t);
      return t;
    },
    clearTimer(h) {
      (h as FakeTimer).cleared = true;
    },
  };
  return {
    children,
    runs,
    delivered,
    timers,
    setShow(ref: string, stdout: string) {
      shows.set(ref, stdout);
    },
    follower: createFollower(deps),
    setInbox(f: () => string) {
      inbox = f;
    },
    holdReads() {
      held = [];
    },
    release(stdout: string) {
      held?.shift()?.(stdout);
    },
    fire() {
      const t = timers.find((x) => !x.cleared);
      if (!t) throw new Error("no pending timer");
      t.cleared = true;
      t.fn();
    },
  };
}

function emptyInbox(): string {
  return JSON.stringify({ kata_api_version: 1, recipient: ADDRESS, requests: [] });
}

function inboxWith(...requests: Array<Record<string, unknown>>): string {
  return JSON.stringify({ kata_api_version: 1, recipient: ADDRESS, requests });
}

function slotLine(id: number, to: Record<string, unknown> | null, key = KEY): string {
  return JSON.stringify({
    event_id: id,
    type: "issue.metadata_updated",
    project_name: "demo",
    issue_short_id: "3909",
    payload: { diff: { [key]: { from: null, to } } },
  });
}

const hello = { from: "dangerboi", message: "hello there" };

function showJson(ref: string, title: string, links: unknown[]): string {
  return JSON.stringify({ kata_api_version: 1, issue: { short_id: ref.slice(ref.indexOf("#") + 1), title }, links });
}

function endpoint(ref: string, status: string) {
  return { qualified_id: ref, short_id: ref.slice(ref.indexOf("#") + 1), project: "demo", status };
}

/** A link as `kata show --json` lists it: `from` blocks `to`, or is related to or the parent of it. */
function link(type: string, from: string, to: string, fromStatus = "open") {
  return { type, from: endpoint(from, fromStatus), to: endpoint(to, "open") };
}

function eventLine(id: number, type: string, ref: string, afterStart: boolean, payload: Record<string, unknown> = {}): string {
  const [project, short] = ref.split("#");
  return JSON.stringify({
    event_id: id,
    type,
    project_name: project,
    issue_short_id: short,
    created_at: new Date(NOW + (afterStart ? 5000 : -5000)).toISOString(),
    payload,
  });
}

describe("blocker closes", () => {
  test("a blocker's close after start delivers once with its title and the blockers still open", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF), link("blocks", "demo#bbbb", REF), link("blocks", "demo#cccc", REF, "closed")]));
    h.follower.start();
    await flush();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF, "closed"), link("blocks", "demo#bbbb", REF), link("blocks", "demo#cccc", REF, "closed")]));
    h.setShow("demo#aaaa", showJson("demo#aaaa", "Ship the parser", []));
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toEqual([`demo#aaaa closed: Ship the parser. ${REF} may be unblocked. 1 other blocker is still open.`]);
  });

  test("the last blocker closing names no remaining count", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF)]));
    h.follower.start();
    await flush();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF, "closed")]));
    h.setShow("demo#aaaa", showJson("demo#aaaa", "Ship the parser", []));
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toEqual([`demo#aaaa closed: Ship the parser. ${REF} may be unblocked.`]);
  });

  test("a close replayed from before the start delivers nothing", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF)]));
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", false));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("a parent, related or unrelated Issue closing delivers nothing", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("parent", "demo#pppp", REF), link("related", "demo#rrrr", REF), link("blocks", REF, "demo#dddd")]));
    h.follower.start();
    await flush();
    for (const [i, ref] of ["demo#pppp", "demo#rrrr", "demo#dddd", "demo#zzzz"].entries()) {
      h.children[0].push(eventLine(30 + i, "issue.closed", ref, true));
    }
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("a blocker added by links_changed delivers when it closes later", async () => {
    const h = harness();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toEqual([]);
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF)]));
    h.children[0].push(eventLine(31, "issue.links_changed", REF, true, { blocked_by_added: ["aaaa"] }));
    await flush();
    h.children[0].push(eventLine(32, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toHaveLength(1);
    expect(h.delivered[0]).toContain("demo#aaaa closed");
  });

  test("a blocker removed by links_changed no longer delivers", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF)]));
    h.follower.start();
    await flush();
    h.setShow(REF, showJson(REF, "mine", []));
    h.children[0].push(eventLine(30, "issue.links_changed", REF, true, { blocked_by_removed: ["aaaa"] }));
    h.children[0].push(eventLine(31, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("a restart with the cursor delivers a close missed in the gap and none already delivered", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF), link("blocks", "demo#bbbb", REF)]));
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toHaveLength(1);
    h.children[0].exit(1);
    await flush();
    h.fire();
    await flush();
    expect(h.children[1].args.slice(-2)).toEqual(["--last-event-id", "30"]);
    h.children[1].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    h.children[1].push(eventLine(31, "issue.closed", "demo#bbbb", true));
    await flush();
    expect(h.delivered).toHaveLength(2);
    expect(h.delivered[1]).toContain("demo#bbbb closed");
  });

  test("a blocker closed, reopened and closed again delivers each close", async () => {
    const h = harness();
    h.setShow(REF, showJson(REF, "mine", [link("blocks", "demo#aaaa", REF)]));
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(30, "issue.closed", "demo#aaaa", true));
    h.children[0].push(eventLine(31, "issue.reopened", "demo#aaaa", true));
    h.children[0].push(eventLine(32, "issue.closed", "demo#aaaa", true));
    await flush();
    expect(h.delivered).toHaveLength(2);
  });
});

describe("comment forwarding", () => {
  const person = { comment_uid: "01JCOMMENT0000000000000001", author: "dana", body: "please also cover the empty case" };
  const on = () => harness(() => emptyInbox(), { forwardComments: true });

  test("a person's comment after start delivers once, with its author, excerpt and UID", async () => {
    const h = on();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", REF, true, person));
    h.children[0].push(eventLine(40, "issue.commented", REF, true, person));
    await flush();
    expect(h.delivered).toEqual([`New comment on ${REF} from dana: please also cover the empty case (c:01JCOMMENT0000000000000001)`]);
  });

  test("a long body is cut to its first lines and a bounded length", async () => {
    const h = on();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", REF, true, { ...person, body: "one\ntwo\nthree\nfour" }));
    h.children[0].push(eventLine(41, "issue.commented", REF, true, { ...person, body: "x".repeat(900) }));
    await flush();
    expect(h.delivered[0]).toBe(`New comment on ${REF} from dana: one\ntwo\nthree\n… (c:${person.comment_uid})`);
    expect(h.delivered[1]).toBe(`New comment on ${REF} from dana: ${"x".repeat(400)}… (c:${person.comment_uid})`);
  });

  test("a comment with a teammate, the agent's own or another's, delivers nothing", async () => {
    const h = on();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", REF, true, { ...person, teammate: "omp-123" }));
    h.children[0].push(eventLine(41, "issue.commented", REF, true, { ...person, teammate: "codex-9" }));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("a comment on another Issue delivers nothing", async () => {
    const h = on();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", "demo#other", true, person));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("a comment replayed from before the start delivers nothing", async () => {
    const h = on();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", REF, false, person));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("off by default: nothing is delivered", async () => {
    const h = harness();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", REF, true, person));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("after a stream restart the comment is not delivered twice, and a newer one is", async () => {
    const h = on();
    h.follower.start();
    await flush();
    h.children[0].push(eventLine(40, "issue.commented", REF, true, person));
    await flush();
    h.children[0].exit();
    await flush();
    h.fire();
    await flush();
    expect(h.children[1].args).toContain("40");
    h.children[1].push(eventLine(40, "issue.commented", REF, true, person));
    h.children[1].push(eventLine(41, "issue.commented", REF, true, { ...person, comment_uid: "01JCOMMENT0000000000000002" }));
    await flush();
    expect(h.delivered).toHaveLength(2);
    expect(h.delivered[1]).toContain("(c:01JCOMMENT0000000000000002)");
  });
});

describe("createFollower", () => {
  test("a trusted slot write delivers once with the delivery text", async () => {
    const h = harness();
    h.follower.start();
    h.children[0].push(slotLine(5, hello));
    await flush();
    expect(h.delivered).toEqual([
      `Message from dangerboi on ${REF}, sent to you through kata:\n\nhello there\n\n` +
        `When you have handled it, clear it: \`kata notify ${REF} --to ${ADDRESS} --clear\``,
    ]);
    expect(h.children[0].args).toEqual(["events", "--tail", "--json", "--project", "demo"]);
    expect(h.runs).toEqual([["inbox", "--for", ADDRESS, "--json", "--project", "demo"]]);
  });

  test("re and kind appear in the delivery text", async () => {
    const h = harness();
    h.follower.start();
    h.children[0].push(slotLine(5, { ...hello, re: "c7", kind: "review" }));
    await flush();
    expect(h.delivered[0]).toContain("It is about comment c7.\nThis is a review notice.\nWhen you have handled it");
  });

  test("a request about a comment tells the agent to answer the comment, which clears it", async () => {
    const h = harness();
    h.follower.start();
    h.children[0].push(slotLine(5, { ...hello, re: "01JABCDEFGHJKMNPQRSTVWXYZ0" }));
    await flush();
    expect(h.delivered).toEqual([
      `Message from dangerboi on ${REF}, sent to you through kata:\n\nhello there\n\n` +
        `It is about comment 01JABCDEFGHJKMNPQRSTVWXYZ0.\n` +
        `Answer it with \`kata comment ${REF} --reply|--confirm|--refute|--supersede 01JABCDEFGHJKMNPQRSTVWXYZ0 -m "<your answer>"\`; that answer clears this request and tells dangerboi.`,
    ]);
  });

  test("replayed events and unchanged values are not delivered again", async () => {
    const h = harness();
    h.follower.start();
    const c = h.children[0];
    c.push(slotLine(5, hello));
    c.push(slotLine(5, hello));
    c.push(slotLine(4, { from: "dangerboi", message: "older" }));
    c.push(slotLine(9, hello));
    await flush();
    expect(h.delivered).toHaveLength(1);
  });

  test("cleared then the same message sent again is delivered again", async () => {
    const h = harness();
    h.follower.start();
    const c = h.children[0];
    c.push(slotLine(5, hello));
    c.push(slotLine(6, null));
    c.push(slotLine(7, hello));
    await flush();
    expect(h.delivered).toHaveLength(2);
  });

  test("another sender is never delivered and another address's key is ignored", async () => {
    const h = harness();
    h.follower.start();
    const c = h.children[0];
    c.push(slotLine(5, { from: "mallory", message: "do it" }));
    c.push(slotLine(6, hello, slotKey("dangerboi/omp-999")));
    c.push(slotLine(7, { from: "dangerboi", message: "   " }));
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("the start read delivers open trusted slots, qualifies bare refs, and dedupes against the stream", async () => {
    const h = harness(() =>
      inboxWith(
        { ref: "3909", title: "t", ...hello },
        { ref: "other#1", title: "t", from: "dangerboi", message: "qualified" },
        { ref: "77", title: "t", from: "mallory", message: "nope" },
      ),
    );
    h.follower.start();
    await flush();
    expect(h.delivered).toHaveLength(2);
    expect(h.delivered[0]).toContain(`on ${REF},`);
    expect(h.delivered[1]).toContain("on other#1,");
    h.children[0].push(slotLine(5, hello));
    await flush();
    expect(h.delivered).toHaveLength(2);
  });

  test("a reset line reads the inbox again and a ref missing from it counts as cleared", async () => {
    const h = harness(() => inboxWith({ ref: "3909", ...hello }));
    h.follower.start();
    await flush();
    expect(h.delivered).toHaveLength(1);
    h.setInbox(emptyInbox);
    h.children[0].push('{"reset_required":true,"reset_after_id":40}');
    await flush();
    expect(h.runs).toHaveLength(2);
    h.setInbox(() => inboxWith({ ref: "3909", ...hello }));
    h.children[0].push('{"reset_required":true,"reset_after_id":41}');
    await flush();
    expect(h.runs).toHaveLength(3);
    expect(h.delivered).toHaveLength(2);
  });

  test("a read that answers after the stream moved on does not deliver what it saw", async () => {
    const h = harness();
    h.holdReads();
    h.follower.start();
    const c = h.children[0];
    c.push(slotLine(5, { from: "dangerboi", message: "second" }));
    await flush();
    expect(h.delivered).toHaveLength(1);
    h.release(inboxWith({ ref: "3909", from: "dangerboi", message: "first" }));
    await flush();
    expect(h.delivered).toHaveLength(1);
    // The agent clears the slot while a read that still saw it is out: the read delivers nothing again.
    c.push('{"reset_required":true,"reset_after_id":6}');
    await flush();
    c.push(slotLine(7, null));
    await flush();
    h.release(inboxWith({ ref: "3909", from: "dangerboi", message: "second" }));
    await flush();
    expect(h.delivered).toHaveLength(1);
  });

  test("a failed inbox read changes nothing", async () => {
    const h = harness(() => "not json");
    h.follower.start();
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("the child exiting restarts it after the backoff from the largest id seen and reads the inbox again", async () => {
    const h = harness();
    h.follower.start();
    h.children[0].push(slotLine(12, hello));
    h.children[0].push(slotLine(8, { from: "dangerboi", message: "older id" }));
    await flush();
    h.children[0].exit(1);
    await flush();
    expect(h.children).toHaveLength(1);
    expect(h.timers.at(-1)?.ms).toBe(1000);
    h.fire();
    await flush();
    expect(h.children).toHaveLength(2);
    expect(h.children[1].args).toEqual([
      "events", "--tail", "--json", "--project", "demo", "--last-event-id", "12",
    ]);
    expect(h.runs).toHaveLength(2);
  });

  test("the backoff doubles up to 30 s while nothing is read and resets after a line", async () => {
    const h = harness();
    h.follower.start();
    const seen: number[] = [];
    for (let i = 0; i < 7; i++) {
      h.children.at(-1)?.exit(1);
      await flush();
      seen.push(h.timers.at(-1)?.ms ?? -1);
      h.fire();
      await flush();
    }
    expect(seen).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000]);
    h.children.at(-1)?.push(slotLine(3, hello));
    await flush();
    h.children.at(-1)?.exit(1);
    await flush();
    expect(h.timers.at(-1)?.ms).toBe(1000);
  });

  test("stop kills the child and nothing restarts", async () => {
    const h = harness();
    h.follower.start();
    await flush();
    h.follower.stop();
    await flush();
    expect(h.children[0].killed).toBe(true);
    expect(h.timers.filter((t) => !t.cleared)).toEqual([]);
    expect(h.children).toHaveLength(1);
  });

  test("stop cancels a pending restart", async () => {
    const h = harness();
    h.follower.start();
    h.children[0].exit(1);
    await flush();
    h.follower.stop();
    expect(h.timers.filter((t) => !t.cleared)).toEqual([]);
    expect(h.children).toHaveLength(1);
  });

  test("a second start while running changes nothing; start after stop spawns again keeping delivered state", async () => {
    const h = harness();
    h.follower.start();
    h.follower.start();
    expect(h.children).toHaveLength(1);
    h.children[0].push(slotLine(5, hello));
    await flush();
    h.follower.stop();
    h.setInbox(() => inboxWith({ ref: "3909", ...hello }));
    h.follower.start();
    await flush();
    expect(h.children).toHaveLength(2);
    h.children[1].push(slotLine(6, hello));
    await flush();
    expect(h.delivered).toHaveLength(1);
  });
});

describe("readConfig", () => {
  test("reads the address and project", () => {
    expect(readConfig({ KATA_INBOX_USER: ADDRESS, KATA_REF: "tern-kata#n4ta" })).toEqual({
      address: ADDRESS,
      project: "tern-kata",
      ref: "tern-kata#n4ta",
    });
  });

  test("TERN_KATA_FORWARD_COMMENTS=1 turns comment forwarding on, anything else leaves it off", () => {
    const base = { KATA_INBOX_USER: ADDRESS, KATA_REF: "p#1" };
    expect(readConfig({ ...base, TERN_KATA_FORWARD_COMMENTS: "1" })?.forwardComments).toBe(true);
    expect(readConfig({ ...base, TERN_KATA_FORWARD_COMMENTS: "0" })?.forwardComments).toBeUndefined();
    expect(readConfig(base)?.forwardComments).toBeUndefined();
  });

  test("blank address, missing project or an address without / yields nothing", () => {
    expect(readConfig({ KATA_REF: "p#1" })).toBeUndefined();
    expect(readConfig({ KATA_INBOX_USER: "  ", KATA_REF: "p#1" })).toBeUndefined();
    expect(readConfig({ KATA_INBOX_USER: ADDRESS })).toBeUndefined();
    expect(readConfig({ KATA_INBOX_USER: ADDRESS, KATA_REF: "nohash" })).toBeUndefined();
    expect(readConfig({ KATA_INBOX_USER: ADDRESS, KATA_REF: "#abc" })).toBeUndefined();
    expect(readConfig({ KATA_INBOX_USER: "solo", KATA_REF: "p#1" })).toBeUndefined();
  });
});

describe("extension factory", () => {
  type Handler = (
    event: unknown,
    ctx: { agent?: { kind?: string }; sessionManager?: { getSessionId?(): string } },
  ) => void;

  function wire(env: Record<string, string | undefined>, stdout = emptyInbox()) {
    const handlers = new Map<string, Handler>();
    const spawned: string[][] = [];
    const sent: Array<{ text: string; options: unknown }> = [];
    const io: KataIo = {
      spawn(args) {
        spawned.push(args);
        return fakeChild(args);
      },
      async run() {
        return { code: 0, stdout };
      },
    };
    const pi: ExtensionApi = {
      on: (event: string, handler: Handler) => void handlers.set(event, handler),
      sendUserMessage: (text, options) => void sent.push({ text, options }),
    };
    createExtension(env, io)(pi);
    return { handlers, spawned, sent };
  }

  const env = { KATA_INBOX_USER: ADDRESS, KATA_REF: "demo#abcd" };

  test("a main session starts the follower and shutdown stops it", async () => {
    const w = wire(env);
    w.handlers.get("session_start")?.({}, {});
    expect(w.spawned).toHaveLength(1);
    w.handlers.get("session_start")?.({}, {});
    expect(w.spawned).toHaveLength(1);
    w.handlers.get("session_shutdown")?.({}, {});
    w.handlers.get("session_start")?.({}, { agent: { kind: "main" } });
    expect(w.spawned).toHaveLength(2);
  });

  test("a subagent session starts nothing", () => {
    const w = wire(env);
    w.handlers.get("session_start")?.({}, { agent: { kind: "sub" } });
    expect(w.spawned).toEqual([]);
  });

  test("no address, no project or an address without / spawns nothing", () => {
    for (const bad of [
      {},
      { KATA_INBOX_USER: ADDRESS },
      { KATA_INBOX_USER: ADDRESS, KATA_REF: "nohash" },
      { KATA_INBOX_USER: "solo", KATA_REF: "demo#abcd" },
    ]) {
      const w = wire(bad);
      w.handlers.get("session_start")?.({}, {});
      expect(w.spawned).toEqual([]);
    }
  });

  test("deliveries go through sendUserMessage as aside", async () => {
    const w = wire(env, inboxWith({ ref: "abcd", ...hello }));
    w.handlers.get("session_start")?.({}, {});
    await flush();
    expect(w.sent).toHaveLength(1);
    expect(w.sent[0].options).toEqual({ deliverAs: "aside" });
    expect(w.sent[0].text).toContain("on demo#abcd,");
  });

  const session = (id: string, kind?: string) => ({ agent: kind ? { kind } : undefined, sessionManager: { getSessionId: () => id } });

  test("each session event names the main session to kata close, without an inbox address too", () => {
    for (const env of [{} as Record<string, string | undefined>, { KATA_INBOX_USER: ADDRESS, KATA_REF: "demo#abcd" }]) {
      const w = wire(env);
      w.handlers.get("session_start")?.({}, session("01a1aade-a1ad-7230-ae16-042ff12a5d53"));
      expect(env.KATA_TRANSCRIPT_AGENT).toBe("omp");
      expect(env.KATA_TRANSCRIPT_SESSION_ID).toBe("01a1aade-a1ad-7230-ae16-042ff12a5d53");
      w.handlers.get("session_switch")?.({}, session("second"));
      expect(env.KATA_TRANSCRIPT_SESSION_ID).toBe("second");
      w.handlers.get("session_branch")?.({}, session("third"));
      expect(env.KATA_TRANSCRIPT_SESSION_ID).toBe("third");
    }
  });

  test("a subagent's session leaves the parent's named", () => {
    const env: Record<string, string | undefined> = {};
    const w = wire(env);
    w.handlers.get("session_start")?.({}, session("parent"));
    w.handlers.get("session_start")?.({}, session("child", "sub"));
    expect(env.KATA_TRANSCRIPT_SESSION_ID).toBe("parent");
  });

  test("a session id kata would refuse, or none, unsets the names so no close cites the session before", () => {
    for (const ctx of [session("has space"), session("-lead"), session("x".repeat(129)), {}]) {
      const env: Record<string, string | undefined> = { KATA_TRANSCRIPT_AGENT: "omp", KATA_TRANSCRIPT_SESSION_ID: "old" };
      wire(env).handlers.get("session_switch")?.({}, ctx);
      expect(env.KATA_TRANSCRIPT_AGENT).toBeUndefined();
      expect(env.KATA_TRANSCRIPT_SESSION_ID).toBeUndefined();
    }
  });
});

describe("session path file", () => {
  type Handler = (event: unknown, ctx: { agent?: { kind?: string }; sessionManager?: { getSessionFile?(): string | undefined } }) => void;

  function wired(argv: string[]) {
    const handlers = new Map<string, Handler>();
    const io: KataIo = {
      spawn: (args) => fakeChild(args),
      async run() {
        return { code: 0, stdout: "" };
      },
    };
    const pi: ExtensionApi = {
      on: (event: string, handler: Handler) => void handlers.set(event, handler),
      sendUserMessage: () => {},
    };
    createExtension({}, io, argv)(pi);
    return handlers;
  }

  function withFolder(body: (dir: string) => void) {
    const dir = mkdtempSync(join(tmpdir(), "tern-kata-session-"));
    try {
      body(dir);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  }

  const session = (file: string | undefined) => ({ sessionManager: { getSessionFile: () => file } });

  test("session start and every switch or branch write the session file's path beside the prompt file", () => {
    withFolder((dir) => {
      const prompt = join(dir, "prompt-abc.md");
      const handlers = wired(["omp", "-e", "x.ts", `@${prompt}`]);
      handlers.get("session_start")?.({}, session("/s/one.jsonl"));
      expect(readFileSync(`${prompt}.session`, "utf8").trim()).toBe("/s/one.jsonl");
      handlers.get("session_switch")?.({}, session("/s/two.jsonl"));
      expect(readFileSync(`${prompt}.session`, "utf8").trim()).toBe("/s/two.jsonl");
      handlers.get("session_branch")?.({}, session("/s/three.jsonl"));
      expect(readFileSync(`${prompt}.session`, "utf8").trim()).toBe("/s/three.jsonl");
      expect(readdirSync(dir).sort()).toEqual(["prompt-abc.md.session"]);
    });
  });

  test("an agent start writes the session file's path only when it changed, as after omp's /move", () => {
    withFolder((dir) => {
      const prompt = join(dir, "prompt-abc.md");
      const handlers = wired([`@${prompt}`]);
      handlers.get("session_start")?.({}, session("/s/one.jsonl"));
      rmSync(`${prompt}.session`);
      handlers.get("agent_start")?.({}, session("/s/one.jsonl"));
      expect(existsSync(`${prompt}.session`)).toBe(false);
      handlers.get("agent_start")?.({}, session("/s/moved.jsonl"));
      expect(readFileSync(`${prompt}.session`, "utf8").trim()).toBe("/s/moved.jsonl");
      handlers.get("agent_start")?.({}, { agent: { kind: "sub" }, ...session("/s/sub.jsonl") });
      expect(readFileSync(`${prompt}.session`, "utf8").trim()).toBe("/s/moved.jsonl");
    });
  });

  test("a subagent's session writes nothing", () => {
    withFolder((dir) => {
      const prompt = join(dir, "prompt-abc.md");
      const handlers = wired([`@${prompt}`]);
      handlers.get("session_start")?.({}, { agent: { kind: "sub" }, ...session("/s/sub.jsonl") });
      expect(existsSync(`${prompt}.session`)).toBe(false);
    });
  });

  test("no prompt file on the command line, or a session without a file, writes nothing", () => {
    withFolder((dir) => {
      const prompt = join(dir, "prompt-abc.md");
      wired(["omp"]).get("session_start")?.({}, session("/s/one.jsonl"));
      wired([`@${prompt}`]).get("session_start")?.({}, session(undefined));
      expect(readdirSync(dir)).toEqual([]);
    });
  });

  test("the prompt file is the last @file named prompt-*.md", () => {
    expect(promptFileFromArgv(["omp", "@notes.txt", "@/state/agent-prompts/prompt-12-ab.md"])).toBe("/state/agent-prompts/prompt-12-ab.md");
    expect(promptFileFromArgv(["omp", "@C:\\state\\prompt-ab.md"])).toBe("C:\\state\\prompt-ab.md");
    expect(promptFileFromArgv(["omp", "@notes.md"])).toBeUndefined();
  });
});
