--!strict

-- What the worker widget shows, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/workersummary_test.luau`. host.luau decodes the output of `tern-kata worker status --json` (internal/worker/status.go's
-- Status, whose JSON keys are its Go field names) and stores `of`'s answer in tern.kv for the window half to read.

export type Held = { ref: string, job: string, reason: string }
export type Summary = {
    executor: string,
    pid: number,
    -- Runs that have not ended and are not held.
    running: number,
    -- Slots summed over the worker's Projects.
    capacity: number,
    inUse: number,
    -- The held runs, in the worker's order.
    held: { Held },
}

local M = {}

-- The `tern.kv` key the host half stores the summary under and the window half reads it from.
M.KEY = "worker.summary"

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

-- The summary of a decoded status, or nil when `status` is not a table that names a worker.
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
            table.insert(summary.held, { ref = text(run.Issue), job = text(run.Job), reason = text(run.Hold) })
        elseif not ended(run) then
            summary.running += 1
        end
    end
    return summary
end

return M
