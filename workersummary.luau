--!strict

-- The summary the host half stores in tern.kv for the worker widget, built as plain data: no Tern call is made here, so the
-- rule is tested by `luau test/workersummary_test.luau`. host.luau decodes the output of `tern-kata worker status --json`
-- (internal/worker/status.go's Status, whose JSON keys are its Go field names), turns `of`'s answer into a `record` stamped
-- with the time on every poll and stores it; the window half reads it back through `fresh`, which drops a record no host
-- half has restamped for a minute, so a worker stopped while no daemon runs on that machine does not stay on the widget.

export type Held = { ref: string, job: string, reason: string }
export type Summary = {
    executor: string,
    pid: number,
    -- Runs that have not ended and are not held.
    running: number,
    -- Slots summed over the worker's Projects.
    capacity: number,
    inUse: number,
    -- The held runs, in the worker's order; each reason is the worker's one-line sentence for the hold.
    held: { Held },
}

export type Record = { at: number, worker: Summary }

local M = {}

M.KEY = "worker.summary"

-- Seconds after which a summary that no host half has restamped is not shown any more. That is several of the host half's polls (POLL_MS in host.luau).
M.STALE_S = 60

local function list(value: any): { any }
    return if type(value) == "table" then value else {}
end

local function text(value: any): string
    return if type(value) == "string" then value else ""
end

local function count(value: any): number
    return if type(value) == "number" then value else 0
end

-- A run that ended has an EndedAt (the worker's zero time, year 1, when it has not); the status lists a few settled runs too, which
-- are neither running nor held.
local function ended(run: any): boolean
    local at = text(run.EndedAt)
    return at ~= "" and at:sub(1, 4) ~= "0001"
end

function M.of(status: any): Summary?
    if type(status) ~= "table" or type(status.PID) ~= "number" then return nil end
    local summary: Summary = {
        executor = text(status.Executor),
        pid = status.PID,
        running = 0,
        capacity = 0,
        inUse = 0,
        held = {},
    }
    for _, project in list(status.Projects) do
        summary.capacity += count(project.Capacity)
        summary.inUse += count(project.InUse)
    end
    for _, run in list(status.Runs) do
        if type(run) ~= "table" then continue end
        if run.Held == true then
            -- HoldLine is the worker's own sentence for the hold; a binary from before it existed gives only the code.
            local reason = text(run.HoldLine)
            if reason == "" then reason = text(run.Hold) end
            table.insert(summary.held, { ref = text(run.Issue), job = text(run.Job), reason = reason })
        elseif not ended(run) then
            summary.running += 1
        end
    end
    return summary
end

function M.record(summary: Summary, now: number): Record
    return { at = now, worker = summary }
end

-- The summary in a record read back from tern.kv when it is still vouched for at `now`, else nil: a record whose `at` is
-- missing or older than STALE_S seconds was left by a host half that no longer polls (no Tern daemon runs on that machine),
-- so its worker may be long gone. `saved` comes from storage, so its shape is checked.
function M.fresh(saved: any, now: number): Summary?
    if type(saved) ~= "table" or type(saved.at) ~= "number" or type(saved.worker) ~= "table" then return nil end
    if now - saved.at > M.STALE_S then return nil end
    return saved.worker
end

return M
