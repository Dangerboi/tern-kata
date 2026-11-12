--!strict

-- Tests of the worker summary the host half stores for the worker widget, on plain data, outside Tern: `luau test/workersummary_test.luau`.
-- The sample is what `tern-kata worker status --json` prints: Go field names, a zero EndedAt for a run that has not ended.

local workersummary = require("../workersummary")

local failures = 0
local function test(name: string, body: () -> ())
    local ok, err = (pcall :: any)(body)
    if ok then
        print("ok   " .. name)
    else
        failures += 1
        print("FAIL " .. name .. ": " .. tostring(err))
    end
end

local ZERO = "0001-01-01T00:00:00Z"

local function run(job: string, issue: string, state: string, held: boolean, hold: string?, ended: string?): { [string]: any }
    return { Job = job, Issue = issue, State = state, Held = held, Hold = hold or "", EndedAt = ended or ZERO }
end

local sample = {
    Executor = "build-box",
    PID = 4242,
    Projects = {
        { Name = "a", Capacity = 3, InUse = 1, Admitting = true },
        { Name = "b", Capacity = 2, InUse = 2, Admitting = true },
    },
    Runs = {
        run("review", "kata-a#12", "held: agent_login", true, "agent_login"),
        run("build", "kata-a#13", "running", false),
        run("sweep", "kata-b#4", "resuming", false),
        run("old", "kata-a#9", "succeeded", false, nil, "2026-11-12T08:00:00Z"),
        run("triage", "kata-b#7", "held: interrupted", true, "interrupted"),
    },
}

test("a status becomes the executor, the pid and the slot sums", function()
    local got = workersummary.of(sample)
    assert(got ~= nil)
    assert(got.executor == "build-box" and got.pid == 4242, got.executor)
    assert(got.capacity == 5 and got.inUse == 3, got.capacity .. "/" .. got.inUse)
end)

test("runs that ended and held runs are not running", function()
    local got = workersummary.of(sample)
    assert(got ~= nil and got.running == 2, tostring(got and got.running))
end)

test("held runs keep the worker's order with their Issue, job and reason", function()
    local got = workersummary.of(sample)
    assert(got ~= nil and #got.held == 2)
    assert(got.held[1].ref == "kata-a#12" and got.held[1].job == "review" and got.held[1].reason == "agent_login")
    assert(got.held[2].ref == "kata-b#7" and got.held[2].job == "triage" and got.held[2].reason == "interrupted")
end)

test("a worker with no Projects and no runs has zeros and no held runs", function()
    local got = workersummary.of({ Executor = "m", PID = 1 })
    assert(got ~= nil and got.running == 0 and got.capacity == 0 and got.inUse == 0 and #got.held == 0)
end)

test("something that names no worker has no summary", function()
    assert(workersummary.of(nil) == nil)
    assert(workersummary.of("text") == nil)
    assert(workersummary.of({ Executor = "m" }) == nil)
end)

test("a held reason is the worker's HoldLine, or the hold code from a binary that has none", function()
    local runs: { any } = {
        { Issue = "k#1", Job = "j", Held = true, Hold = "secret_unavailable", HoldLine = "secret_unavailable: no token", EndedAt = ZERO },
        { Issue = "k#2", Job = "j", Held = true, Hold = "interrupted", EndedAt = ZERO },
    }
    local got = workersummary.of({ PID = 1, Runs = runs })
    assert(got ~= nil and #got.held == 2)
    assert(got.held[1].reason == "secret_unavailable: no token", got.held[1].reason)
    assert(got.held[2].reason == "interrupted", got.held[2].reason)
end)

test("a summary is fresh until STALE_S seconds after its stamp", function()
    local summary = workersummary.of(sample)
    assert(summary ~= nil)
    summary.at = 1000
    assert(workersummary.fresh(summary, 1000) == summary)
    assert(workersummary.fresh(summary, 1000 + workersummary.STALE_S) == summary)
    assert(workersummary.fresh(summary, 1000 + workersummary.STALE_S + 1) == nil)
end)

test("a summary without a numeric stamp, or none at all, is not fresh", function()
    local summary = workersummary.of(sample)
    assert(summary ~= nil)
    local loose: any = summary
    loose.at = nil
    assert(workersummary.fresh(summary, 1000) == nil)
    assert(workersummary.fresh(nil, 1000) == nil)
end)

if failures > 0 then error(failures .. " failed") end
