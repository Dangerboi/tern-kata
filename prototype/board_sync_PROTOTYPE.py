#!/usr/bin/env python3
"""PROTOTYPE, throwaway. Question: can Tern's own board block drive kata through its Markdown file?

Keeps one board file and one kata Project in step:
  kata -> file: every Issue is a task line "Title (id)" under its Lane heading.
  file -> kata: a task line that changed Lane becomes a Move (label swap, close, reopen);
                a line with no id becomes a new Issue in that Lane.
Polls both sides; no events, no error handling beyond keeping the loop alive.

Run: python3 prototype/board_sync_PROTOTYPE.py BOARD_FILE --workspace DIR
"""
import json
import os
import re
import subprocess
import sys
import time

# Lane heading -> label. None: No lane (open, no Lane label). "DONE": closed Issues.
LANES = [("No lane", None), ("In progress", "in-progress"), ("In review", "in-review"), ("Done", "DONE")]
LANE_LABELS = {label for _, label in LANES if label not in (None, "DONE")}
ID = re.compile(r"\(([a-z0-9]{4})\)\s*$")
TASK = re.compile(r"^- \[( |x)\] (.*)$")

board, workspace = sys.argv[1], sys.argv[sys.argv.index("--workspace") + 1]


def kata(*args):
    r = subprocess.run(["kata", "--workspace", workspace, *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"kata {' '.join(args)}: {r.stdout.strip() or r.stderr.strip()}")
    return json.loads(r.stdout) if "--json" in args else r.stdout


def lane_of(issue):
    if issue["status"] == "closed":
        return "Done"
    for title, label in LANES:
        if label in (issue.get("labels") or []):
            return title
    return "No lane"


def issues():
    return {i["short_id"]: i for i in kata("list", "--status", "all", "--limit", "0", "--json")["issues"]}


def parse(text):
    """Lane title -> ordered [(id or None, text)]."""
    lanes, lane = {}, None
    for line in text.splitlines():
        if line.startswith("## "):
            lane = line[3:].strip()
            lanes.setdefault(lane, [])
        elif lane and (m := TASK.match(line)):
            m_id = ID.search(m.group(2))
            lanes[lane].append((m_id.group(1) if m_id else None, m.group(2)))
    return lanes


def render(state, previous):
    """Each Lane keeps the file's existing order; Issues new to a Lane go last."""
    old = parse(previous)
    out = []
    for title, label in LANES:
        out.append(f"## {title}\n")
        if label == "DONE":
            out.append("**Complete**")
        ids = [i for i, _ in old.get(title, []) if i in state and lane_of(state[i]) == title]
        ids += [i for i in state if lane_of(state[i]) == title and i not in ids]
        for i in ids:
            mark = "x" if label == "DONE" else " "
            out.append(f"- [{mark}] {state[i]['title']} ({i})")
        out.append("")
    return "\n".join(out)


def close(issue):
    """The quick close tern-kata's Close card sends: source tui, no evidence (the CLI demands evidence)."""
    sock = kata("daemon", "locate", "--json")["address"].removeprefix("unix://")
    body = json.dumps({"actor": os.environ.get("USER", "board"), "reason": "done", "retry_protocol": "close-v1",
                       "source": "tui", "message": "Moved to Done on the Tern board (prototype sync)."})
    url = f"http://kata/api/v1/projects/{issue['project_id']}/issues/{issue['short_id']}/actions/close"
    r = subprocess.run(["curl", "-s", "--unix-socket", sock, "-X", "POST", url, "-H", "Content-Type: application/json",
                        "-d", body], capture_output=True, text=True)
    if '"error"' in r.stdout:
        raise RuntimeError(r.stdout)


def move(issue_id, issue, target):
    label = dict(LANES)[target]
    if label == "DONE":
        close(issue)
        return
    if issue["status"] == "closed":
        kata("reopen", issue_id)
    if label:
        kata("label", "add", issue_id, label)  # add first: a failed removal leaves two Lanes, never none
    for old in set(issue.get("labels") or []) & LANE_LABELS - {label}:
        kata("label", "rm", issue_id, old)


def write(text):
    tmp = board + ".sync-tmp"
    with open(tmp, "w") as f:
        f.write(text)
    os.replace(tmp, board)


def show(state, why):
    print(f"\n[{time.strftime('%H:%M:%S')}] {why}")
    for title, _ in LANES:
        names = [f"{i} {state[i]['title']}" for i in state if lane_of(state[i]) == title]
        print(f"  {title:12} {names}")
    sys.stdout.flush()


last_written = None
state = issues()
text = open(board).read() if os.path.exists(board) else ""
last_written = render(state, text)
write(last_written)
show(state, "start: wrote board from kata")

while True:
    time.sleep(0.5)
    try:
        text = open(board).read()
        if text != last_written:
            state = issues()
            # A drag is a Lane change against the file we last wrote, not against kata: the board may save a file
            # written before a kata-side change, and its stale lines must not undo that change.
            given = {i: lane for lane, rows in parse(last_written).items() for i, _ in rows}
            for lane, rows in parse(text).items():
                if lane not in dict(LANES):
                    continue
                for issue_id, line in rows:
                    if issue_id is None:
                        title = line.strip()
                        created = kata("create", title, "--json")["issue"]["short_id"]
                        print(f"  new card {title!r} -> created {created}")
                        move(created, {"status": "open", "labels": []}, lane) if lane != "No lane" else None
                    elif issue_id in state and given.get(issue_id) != lane and lane_of(state[issue_id]) != lane:
                        print(f"  drag {issue_id}: {lane_of(state[issue_id])} -> {lane}")
                        try:
                            move(issue_id, state[issue_id], lane)
                        except RuntimeError as err:
                            print(f"  refused, card goes back: {err}")
            state = issues()
            last_written = render(state, text)
            write(last_written)
            show(state, "board changed: applied to kata, rewrote board")
            continue
        fresh = issues()
        if fresh != state:
            state = fresh
            last_written = render(state, text)
            write(last_written)
            show(state, "kata changed: rewrote board")
    except Exception as err:  # keep the prototype alive
        print(f"  error: {err}")
