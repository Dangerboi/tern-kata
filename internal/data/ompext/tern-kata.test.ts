import { describe, expect, test } from "bun:test";
import {
  createExtension,
  createInbox,
  readConfig,
  slotKey,
  type ExtensionApi,
  type InboxDeps,
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
  let resolveExit: (code: number) => void = () => {};
  const exited = new Promise<number>((r) => (resolveExit = r));
  async function* lines(): AsyncGenerator<string> {
    for (;;) {
      if (queue.length > 0) yield queue.shift() as string;
      else if (ended) return;
      else await new Promise<void>((r) => (wake = r));
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

function harness(inboxStdout: () => string = () => emptyInbox()) {
  const children: FakeChild[] = [];
  const runs: string[][] = [];
  const delivered: string[] = [];
  const timers: FakeTimer[] = [];
  let inbox = inboxStdout;
  const deps: InboxDeps = {
    address: ADDRESS,
    project: "demo",
    spawn(args) {
      const c = fakeChild(args);
      children.push(c);
      return c;
    },
    async run(args) {
      runs.push(args);
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
    inbox: createInbox(deps),
    setInbox(f: () => string) {
      inbox = f;
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

describe("createInbox", () => {
  test("a trusted slot write delivers once with the delivery text", async () => {
    const h = harness();
    h.inbox.start();
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
    h.inbox.start();
    h.children[0].push(slotLine(5, { ...hello, re: "c7", kind: "review" }));
    await flush();
    expect(h.delivered[0]).toContain("It is about comment c7.\nThis is a review notice.\nWhen you have handled it");
  });

  test("replayed events and unchanged values are not delivered again", async () => {
    const h = harness();
    h.inbox.start();
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
    h.inbox.start();
    const c = h.children[0];
    c.push(slotLine(5, hello));
    c.push(slotLine(6, null));
    c.push(slotLine(7, hello));
    await flush();
    expect(h.delivered).toHaveLength(2);
  });

  test("another sender is never delivered and another address's key is ignored", async () => {
    const h = harness();
    h.inbox.start();
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
    h.inbox.start();
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
    h.inbox.start();
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

  test("a failed inbox read changes nothing", async () => {
    const h = harness(() => "not json");
    h.inbox.start();
    await flush();
    expect(h.delivered).toEqual([]);
  });

  test("the child exiting restarts it after the backoff from the largest id seen and reads the inbox again", async () => {
    const h = harness();
    h.inbox.start();
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
    h.inbox.start();
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
    h.inbox.start();
    await flush();
    h.inbox.stop();
    await flush();
    expect(h.children[0].killed).toBe(true);
    expect(h.timers.filter((t) => !t.cleared)).toEqual([]);
    expect(h.children).toHaveLength(1);
  });

  test("stop cancels a pending restart", async () => {
    const h = harness();
    h.inbox.start();
    h.children[0].exit(1);
    await flush();
    h.inbox.stop();
    expect(h.timers.filter((t) => !t.cleared)).toEqual([]);
    expect(h.children).toHaveLength(1);
  });

  test("a second start while running changes nothing; start after stop spawns again keeping delivered state", async () => {
    const h = harness();
    h.inbox.start();
    h.inbox.start();
    expect(h.children).toHaveLength(1);
    h.children[0].push(slotLine(5, hello));
    await flush();
    h.inbox.stop();
    h.setInbox(() => inboxWith({ ref: "3909", ...hello }));
    h.inbox.start();
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
    });
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
  type Handler = (event: unknown, ctx: { agent?: { kind?: string } }) => void;

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
});
