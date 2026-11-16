--!strict

-- Tests of the worker summary the host half stores for the worker widget, on plain data, outside Tern: `luau test/workersummary_test.luau`.
-- The sample is what `tern-kata worker status --json` prints: Go field names, a zero EndedAt for a run that has not ended and a real one for a held run, as the worker's outcome stamps it.

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
        run("review", "kata-a#12", "held: agent_login", true, "agent_login", "2026-11-12T09:30:00Z"),
        run("build", "kata-a#13", "running", false),
        run("sweep", "kata-b#4", "resuming", false),
        run("old", "kata-a#9", "succeeded", false, nil, "2026-11-12T08:00:00Z"),
        run("triage", "kata-b#7", "held: interrupted", true, "interrupted", "2026-11-12T09:31:00Z"),
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

test("held runs keep the worker's order with their Issue and reason", function()
    local got = workersummary.of(sample)
    assert(got ~= nil and #got.held == 2)
    assert(got.held[1].ref == "kata-a#12" and got.held[1].reason == "agent_login")
    assert(got.held[2].ref == "kata-b#7" and got.held[2].reason == "interrupted")
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

test("a Projects entry that is not a table is skipped", function()
    local got = workersummary.of({ PID = 1, Projects = { "text", 7, { Capacity = 2, InUse = 1 } } })
    assert(got ~= nil and got.capacity == 2 and got.inUse == 1)
end)

local function fakeKv(initial: any, failGet: boolean?): (workersummary.Kv, { [string]: any })
    local data: { [string]: any } = { [workersummary.KEY] = initial }
    local kv: workersummary.Kv = {
        get = function(key: string): any
            if failGet then error("kv unreadable") end
            return data[key]
        end,
        set = function(key: string, value: any) data[key] = value end,
    }
    return kv, data
end

test("a saved summary loads back and is fresh until STALE_S seconds after its stamp", function()
    local summary = workersummary.of(sample)
    assert(summary ~= nil)
    local kv = fakeKv(nil)
    assert(workersummary.save(kv, summary, 1000) == nil)
    local record = workersummary.load(kv, nil)
    assert(record ~= nil and record.at == 1000)
    local got = workersummary.fresh(record, 1000 + workersummary.STALE_S)
    assert(got ~= nil and got.executor == "build-box" and got.pid == 4242 and got.running == 2 and got.capacity == 5)
    assert(got.held[2].ref == "kata-b#7" and got.held[2].reason == "interrupted")
    assert(workersummary.fresh(record, 1000 + workersummary.STALE_S + 1) == nil)
    assert(workersummary.fresh(nil, 1000) == nil)
end)

test("a store that refuses the record says why", function()
    local summary = workersummary.of(sample)
    assert(summary ~= nil)
    local kv: workersummary.Kv = { get = function(): any return nil end, set = function() error("disk full") end }
    local err = workersummary.save(kv, summary, 1)
    assert(err ~= nil and err:find("disk full", 1, true) ~= nil, tostring(err))
end)

test("something read back without a numeric stamp or a worker table loads as nothing", function()
    local worker = { executor = "m", pid = 1, running = 0, capacity = 0, inUse = 0, held = {} }
    for _, stored in {{ worker = worker }, { at = "1000", worker = worker }, { at = 1000 }, { at = 1000, worker = "text" }, "text"} do
        assert(workersummary.load(fakeKv(stored), nil) == nil)
    end
    assert(workersummary.load(fakeKv(nil), nil) == nil)
end)

test("a record whose worker has the wrong shape is not shown as it is", function()
    assert(workersummary.load(fakeKv({ at = 1000, worker = {} }), nil) == nil)
    assert(workersummary.load(fakeKv({ at = 1000, worker = { pid = "1" } }), nil) == nil)
    local record = workersummary.load(fakeKv({ at = 1000, worker = { pid = 1, held = { "x", { ref = "k#1", reason = 5 } } } }), nil)
    assert(record ~= nil)
    local worker = record.worker
    assert(worker.executor == "" and worker.running == 0 and worker.capacity == 0 and worker.inUse == 0)
    assert(#worker.held == 1 and worker.held[1].ref == "k#1" and worker.held[1].reason == "")
end)

test("a failed read keeps the record that was kept, and a clean read replaces it", function()
    local kept = { at = 5, worker = { executor = "old", pid = 9, running = 0, capacity = 0, inUse = 0, held = {} } }
    assert(workersummary.load(fakeKv(nil, true), kept) == kept)
    assert(workersummary.load(fakeKv(nil), kept) == nil)
end)

test("drop removes the record and does nothing when there is none", function()
    local kv, data = fakeKv({ at = 1 })
    workersummary.drop(kv)
    assert(data[workersummary.KEY] == nil)
    workersummary.drop(kv)
    workersummary.drop((fakeKv(nil, true)))
end)

if failures > 0 then error(failures .. " test(s) failed") end
