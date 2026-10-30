#!/usr/bin/env python3
"""Check-speed eval runner: times every check unit of scripts/check.sh and grades that it still passes with its coverage.

One rep runs the three groups in turn (static, test, smoke), each through `scripts/check.sh <group>`, after clearing the
Go test cache and the golangci-lint cache (module and build caches stay warm: the state after an edit). A unit's latency_s
is when its verdict is known, counted from the start of its group. Writes results.jsonl, errors.jsonl and train-only
traces under --out, resuming per (group, rep).
"""

import argparse
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
from datetime import datetime
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
CASES = json.loads((HERE / "cases.json").read_text())
COVERAGE = json.loads((HERE / "coverage.json").read_text())
GROUP_TIMEOUT = {"static": 1200, "gotest": 1200, "smoke": 3600}
GROUP_ARG = {"static": "static", "gotest": "test", "smoke": "smoke"}
ASSERTION = re.compile(r"expect_tree |jq -e|hint_expect_mode |\bcmp\b|\|\| \{|\[\[ |grep -q|wait_[a-z_]+ ")


def assertions(path: Path) -> int:
    return len(ASSERTION.findall(path.read_text())) if path.exists() else 0


def lint_config_key() -> str:
    """The .golangci.yml minus its run: block (concurrency, timeouts): what is checked, not how fast."""
    out, skipping = [], False
    for line in (REPO / ".golangci.yml").read_text().splitlines():
        if re.match(r"^\S", line):
            skipping = line.startswith("run:")
        if not skipping and line.strip() and not line.lstrip().startswith("#"):
            out.append(line.rstrip())
    return "\n".join(out)


def lint_kept() -> float:
    enabled = json.loads(subprocess.run(["golangci-lint", "linters", "--json"], cwd=REPO, capture_output=True, text=True).stdout)["Enabled"]
    names = {linter["name"] for linter in enabled}
    same_linters = set(COVERAGE["lint"]["linters"]) <= names
    same_rules = lint_config_key() == COVERAGE["lint"]["config"]
    return 1.0 if same_linters and same_rules else 0.0


def train_ids(state_path: Path) -> set:
    if not state_path.exists():
        return set()
    return set(json.loads(state_path.read_text()).get("train_ids") or [])


def run_group(group: str, out_dir: Path, log_path: Path):
    """Runs one group; returns (t0, [(epoch, line)], exit code or None on timeout)."""
    env = dict(os.environ, CHECK_OUT=str(out_dir))
    t0 = time.time()
    proc = subprocess.Popen(["scripts/check.sh", GROUP_ARG[group]], cwd=REPO, env=env, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, text=True, bufsize=1, start_new_session=True)
    lines, timed_out = [], threading.Event()

    def kill():
        timed_out.set()
        os.killpg(proc.pid, signal.SIGKILL)

    watchdog = threading.Timer(GROUP_TIMEOUT[group], kill)
    watchdog.start()
    with log_path.open("w") as log:
        for line in proc.stdout:
            now = time.time()
            lines.append((now, line.rstrip("\n")))
            log.write(f"{now - t0:9.3f} {line}")
    code = proc.wait()
    watchdog.cancel()
    return t0, lines, None if timed_out.is_set() else code


def excerpt(lines, t0, start, end, limit=400):
    """The log lines between start and end, each with its time since the group began and the gap before it."""
    picked, prev = [], start
    for t, text in lines:
        if start <= t <= end:
            picked.append(f"+{t - t0:8.3f}s (gap {t - prev:6.3f}s) {text[:300]}")
            prev = t
    if len(picked) > limit:
        picked = picked[: limit // 2] + [f"... {len(picked) - limit} lines cut ..."] + picked[-limit // 2:]
    return "\n".join(picked)


def grade_static(t0, lines, code, work: Path):
    marks = {}
    for t, text in lines:
        m = re.match(r"^CHECK (\S+) (ok|FAIL)$", text)
        if m:
            marks[m.group(1)] = (t, m.group(2) == "ok")
    end = lines[-1][0] if lines else t0
    results, prev = {}, t0
    for case in [c for c in CASES if c["tag"] == "static"]:
        unit = case["unit"]
        t, ok = marks.get(unit, (end, False))
        kept = 1.0
        if unit.startswith("lint"):
            kept = lint_kept()
        elif unit == "build":
            kept = 1.0 if (work / "bin" / "tern-kata").exists() else 0.0
        elif unit.startswith("cross-"):
            _, os_, arch = unit.split("-")
            kept = 1.0 if (work / "bin" / f"{os_}_{arch}" / ("tern-kata.exe" if os_ == "windows" else "tern-kata")).exists() else 0.0
        why = "" if unit in marks else "no CHECK line"
        results[case["id"]] = dict(latency=t - t0, ok=ok, kept=kept, why=why, trace=excerpt(lines, t0, min(prev, t), t))
        prev = max(prev, t)
    return results


def grade_gotest(t0, lines, code, work: Path):
    events = []
    stream = work / "gotest.jsonl"
    if stream.exists():
        for raw in stream.read_text().splitlines():
            try:
                events.append(json.loads(raw))
            except json.JSONDecodeError:
                pass
    end = lines[-1][0] if lines else time.time()
    done, passed, elapsed = {}, {}, {}
    for e in events:
        pkg, test, action = e.get("Package", ""), e.get("Test"), e.get("Action")
        if test is None and action in ("pass", "fail"):
            done[pkg] = (datetime.fromisoformat(e["Time"]).timestamp(), action == "pass")
        elif test is not None and action == "pass":
            passed.setdefault(pkg, set()).add(test)
            elapsed.setdefault(pkg, {})[test] = e.get("Elapsed", 0)
    results = {}
    for case in [c for c in CASES if c["tag"] == "gotest"]:
        pkg = case["unit"]
        t, ok = done.get(pkg, (end, False))
        base = set(COVERAGE["gotest"][pkg])
        kept = len(base & passed.get(pkg, set())) / len(base) if base else 1.0
        slow = sorted(elapsed.get(pkg, {}).items(), key=lambda kv: -kv[1])[:25]
        times = elapsed.get(pkg, {})
        top = sum(v for k, v in times.items() if "/" not in k)
        over = sum(1 for k, v in times.items() if "/" not in k and v >= 0.5)
        trace = (f"package {pkg} finished {t - t0:.3f}s after go test started; {len(passed.get(pkg, ()))} tests passed; top-level test Elapsed sums to {top:.1f}s, {over} top-level tests took >= 0.5s\n"
                 "slowest tests (Elapsed s, includes time paused for t.Parallel):\n" + "\n".join(f"{v:8.3f}  {k}" for k, v in slow))
        results[case["id"]] = dict(latency=t - t0, ok=ok, kept=kept, why="" if pkg in done else "no package result", trace=trace)
    return results


def grade_smoke(t0, lines, code, work: Path):
    marks = {}
    for t, text in lines:
        m = re.match(r"^Step passed (\S+)$", text)
        if m:
            marks[m.group(1)] = t
        elif text.startswith("Plugin smoke passed"):
            marks["plugin.sh"] = t
        elif text.startswith("Supervision smoke passed"):
            marks["supervision.sh"] = t
    end = lines[-1][0] if lines else t0
    results, prev = {}, t0
    for case in [c for c in CASES if c["tag"] == "smoke"]:
        unit = case["unit"]
        path = REPO / "smoke" / ("steps/" + unit if unit not in ("plugin.sh", "supervision.sh") else unit)
        ok = unit in marks
        t = marks.get(unit, end)
        base = COVERAGE["smoke"][unit]
        kept = min(1.0, assertions(path) / base) if base else 1.0
        start = prev if ok else t0
        results[case["id"]] = dict(latency=t - t0, ok=ok, kept=kept, why="" if ok else "no pass line (suite stopped before or at this unit)", trace=excerpt(lines, t0, start, t))
        if ok:
            prev = t
    return results


GRADERS = {"static": grade_static, "gotest": grade_gotest, "smoke": grade_smoke}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--reps", type=int, default=2)
    ap.add_argument("--state", default=None, help="ratchet _state.json, read only, for the train split")
    ap.add_argument("--groups", default="static,gotest,smoke")
    args = ap.parse_args()
    out = Path(args.out)
    (out / "traces").mkdir(parents=True, exist_ok=True)
    (out / "logs").mkdir(exist_ok=True)
    state = Path(args.state) if args.state else out.parent / "_state.json"
    train = train_ids(state)
    results_path, errors_path = out / "results.jsonl", out / "errors.jsonl"
    have = set()
    if results_path.exists():
        for raw in results_path.read_text().splitlines():
            row = json.loads(raw)
            have.add((row["prompt_id"], row["rep"]))
    groups = args.groups.split(",")
    for rep in range(args.reps):
        for group in groups:
            ids = [c["id"] for c in CASES if c["tag"] == group]
            if all((i, rep) in have for i in ids):
                continue
            subprocess.run(["go", "clean", "-testcache"], cwd=REPO, check=True)
            subprocess.run(["golangci-lint", "cache", "clean"], cwd=REPO, check=True)
            with tempfile.TemporaryDirectory(prefix=f"check-{group}-") as work:
                t0, lines, code = run_group(group, Path(work), out / "logs" / f"{group}_rep{rep}.log")
                if code is None:
                    with errors_path.open("a") as f:
                        for i in ids:
                            if (i, rep) not in have:
                                f.write(json.dumps({"prompt_id": i, "rep": rep, "class": "timeout", "detail": f"{group} group passed {GROUP_TIMEOUT[group]}s"}) + "\n")
                    continue
                graded = GRADERS[group](t0, lines, code, Path(work))
            with results_path.open("a") as f:
                for case in [c for c in CASES if c["tag"] == group]:
                    if (case["id"], rep) in have:
                        continue
                    g = graded[case["id"]]
                    row = {
                        "prompt_id": case["id"], "rep": rep, "prompt": f"scripts/check.sh {GROUP_ARG[group]} :: {case['unit']}",
                        "tags": [group], "grade": {"pass": 1.0 if g["ok"] else 0.0, "kept": round(g["kept"], 4)},
                        "explanation": g["why"], "status": "ok", "model": "none", "usage": {"input_tokens": 0, "output_tokens": 0},
                        "latency_s": round(g["latency"], 3), "group_exit": code,
                    }
                    f.write(json.dumps(row) + "\n")
                    if case["id"] in train:
                        trace = [{"role": "user", "content": row["prompt"]},
                                 {"role": "tool_result", "name": group, "content": f"pass={g['ok']} kept={g['kept']:.3f} latency_s={g['latency']:.3f} {g['why']}\n{g['trace']}"}]
                        path = out / "traces" / f"{case['id']}_rep{rep}.json"
                        path.parent.mkdir(parents=True, exist_ok=True)
                        path.write_text(json.dumps(trace, indent=1))
    return 0


if __name__ == "__main__":
    sys.exit(main())
