--!strict

-- The summary of the kata worker that the worker widget shows, built as plain data: no Tern call is made here (the kv table is
-- handed in), so the rule is tested by `luau test/workersummary_test.luau`. host.luau decodes the output of `tern-kata worker
-- status --json` (internal/worker/status.go's Status, whose JSON keys are its Go field names) with `of` and renders the
-- widget from it. Where the host half cannot render widgets it also `save`s the summary, stamped with the time of every poll,
-- in tern.kv for the window half's widget, which `load`s it and shows it through `fresh`: that drops a record no host half has
-- restamped for a minute, so a worker stopped while no daemon runs on that machine does not stay on the widget.

-- `uid` is the run's uid (what `tern-kata worker resume` takes), `job` its job's name, `title` its Issue's title as the run
-- started with it, `resumable` and `abandonable` whether the worker would resume or abandon it now, and `heldAt` when it was
-- held (Unix seconds, from its EndedAt); a binary from before those fields gives "", "", "", false, false and nil.
export type Held = {
    uid: string, job: string, ref: string, title: string, reason: string, resumable: boolean, abandonable: boolean, heldAt: number?,
}
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
export type Kv = { get: (key: string) -> any, set: (key: string, value: any) -> () }

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

-- The Unix time of an RFC 3339 stamp as Go writes it (`2026-11-18T09:00:00.5+01:00`, `...Z`); nil for anything else and for
-- the worker's zero time.
function M.epoch(stamp: string): number?
    local y, mo, d, h, mi, s, rest = stamp:match("^(%d%d%d%d)%-(%d%d)%-(%d%d)T(%d%d):(%d%d):(%d%d)(.*)$")
    if y == nil or y == "0001" then return nil end
    local zone = (rest :: string):gsub("^%.%d+", "")
    local offset = 0
    if zone ~= "Z" then
        local sign, oh, om = zone:match("^([+-])(%d%d):(%d%d)$")
        if sign == nil then return nil end
        offset = ((tonumber(oh) :: number) * 60 + (tonumber(om) :: number)) * 60 * (if sign == "-" then -1 else 1)
    end
    -- Days since 1970-01-01 of a civil date (H. Hinnant's days_from_civil).
    local month = tonumber(mo) :: number
    local year = (tonumber(y) :: number) - (if month <= 2 then 1 else 0)
    local era = year // 400
    local yearOfEra = year - era * 400
    local dayOfYear = (153 * ((month + 9) % 12) + 2) // 5 + (tonumber(d) :: number) - 1
    local days = era * 146097 + yearOfEra * 365 + yearOfEra // 4 - yearOfEra // 100 + dayOfYear - 719468
    return days * 86400 + (tonumber(h) :: number) * 3600 + (tonumber(mi) :: number) * 60 + (tonumber(s) :: number) - offset
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
        if type(project) ~= "table" then continue end
        summary.capacity += count(project.Capacity)
        summary.inUse += count(project.InUse)
    end
    for _, run in list(status.Runs) do
        if type(run) ~= "table" then continue end
        -- A held run has an EndedAt too (its outcome's time), so Held is tested before ended.
        if run.Held == true then
            -- HoldLine is the worker's own sentence for the hold; a binary from before it existed gives only the code.
            local reason = text(run.HoldLine)
            if reason == "" then reason = text(run.Hold) end
            table.insert(summary.held, {
                uid = text(run.UID), job = text(run.Job), ref = text(run.Issue), title = text(run.IssueTitle), reason = reason,
                resumable = run.Resumable == true, abandonable = run.Abandonable == true, heldAt = M.epoch(text(run.EndedAt)),
            })
        elseif not ended(run) then
            summary.running += 1
        end
    end
    return summary
end

-- A summary as `save` wrote it, checked field by field because it comes from storage (another version of the plugin may have
-- written it); nil when it names no worker.
local function summaryOf(stored: any): Summary?
    if type(stored) ~= "table" or type(stored.pid) ~= "number" then return nil end
    local summary: Summary = {
        executor = text(stored.executor),
        pid = stored.pid,
        running = count(stored.running),
        capacity = count(stored.capacity),
        inUse = count(stored.inUse),
        held = {},
    }
    for _, run in list(stored.held) do
        if type(run) == "table" then
            table.insert(summary.held, {
                uid = text(run.uid), job = text(run.job), ref = text(run.ref), title = text(run.title), reason = text(run.reason),
                resumable = run.resumable == true, abandonable = run.abandonable == true,
                heldAt = if type(run.heldAt) == "number" then run.heldAt else nil,
            })
        end
    end
    return summary
end

-- Stores the summary stamped `now`; the error text when the store refuses it.
function M.save(kv: Kv, summary: Summary, now: number): string?
    local ok, err = pcall(function(): any? kv.set(M.KEY, { at = now, worker = summary }); return nil end)
    if ok then return nil end
    return tostring(err)
end

-- The record in storage, or `kept` when the read fails (the host half may be rewriting the file; `fresh` still ages the kept
-- record out). Nothing stored, or something that is not a record, gives nil.
function M.load(kv: Kv, kept: Record?): Record?
    local ok, stored = pcall(kv.get, M.KEY)
    if not ok then return kept end
    if type(stored) ~= "table" or type(stored.at) ~= "number" then return nil end
    local worker = summaryOf(stored.worker)
    if worker == nil then return nil end
    return { at = stored.at, worker = worker }
end

-- Removes the record, whichever load stored it, if there is one.
function M.drop(kv: Kv)
    local ok, stored = pcall(kv.get, M.KEY)
    if ok and stored ~= nil then pcall(kv.set, M.KEY, nil) end
end

-- The summary of a record when it is still vouched for at `now`, else nil: a record older than STALE_S seconds was left by a
-- host half that no longer polls (no Tern daemon runs on that machine), so its worker may be long gone.
function M.fresh(record: Record?, now: number): Summary?
    if record == nil or now - record.at > M.STALE_S then return nil end
    return record.worker
end

return M
