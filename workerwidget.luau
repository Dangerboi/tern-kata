--!strict

-- What the kata worker widget in the widget bar shows, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/workerwidget_test.luau`. The host half stores a summary of the worker under the `tern.kv` key `worker.summary`;
-- window.luau reads it on a timer, keeps it, and asks `view` for what to draw and `same` whether it changed.

export type Held = { ref: string, job: string, reason: string }
export type Summary = { executor: string, pid: number, running: number, capacity: number, inUse: number, held: { Held } }
export type WidgetView = { text: string, tone: string?, title: string?, command: string?, value: number? }

local M = {}

local TITLE_HELD = 5

-- The tooltip: which worker this is, then each held run with why it waits (at most five, then how many more), then how many
-- runs go on. The executor and pid are named so a remote window never mistakes this machine's worker for the session host's.
local function title(summary: Summary): string
    local lines = {"Worker " .. summary.executor .. " on this machine (process " .. summary.pid .. ")"}
    for index = 1, math.min(#summary.held, TITLE_HELD) do
        local held = summary.held[index]
        table.insert(lines, held.ref .. " " .. held.job .. ": " .. held.reason)
    end
    if #summary.held > TITLE_HELD then table.insert(lines, "and " .. (#summary.held - TITLE_HELD) .. " more") end
    table.insert(lines, summary.running .. " running")
    return table.concat(lines, "\n")
end

-- What the widget shows: nothing while no worker runs on this machine. Held runs win the text and the warning tone, and a
-- click shows kata's held runs; otherwise a click opens kata. `heldCommand` and `openCommand` are the action names.
function M.view(summary: Summary?, heldCommand: string, openCommand: string): WidgetView?
    if summary == nil then return nil end
    local value = if summary.capacity > 0 then summary.inUse / summary.capacity else 0
    local held = #summary.held
    if held > 0 then
        return {text = held .. " held", tone = "warning", title = title(summary), command = heldCommand, value = value}
    end
    local text = if summary.running > 0 then summary.running .. " running" else "idle"
    return {text = text, tone = "muted", title = title(summary), command = openCommand, value = value}
end

-- A stable text of a summary (nil for none), so the timer can tell whether what it read differs from what it kept.
function M.key(summary: Summary?): string
    if summary == nil then return "" end
    local parts = {summary.executor, tostring(summary.pid), tostring(summary.running), tostring(summary.capacity), tostring(summary.inUse)}
    for _, held in summary.held do
        table.insert(parts, held.ref .. "\0" .. held.job .. "\0" .. held.reason)
    end
    return table.concat(parts, "\1")
end

function M.same(a: Summary?, b: Summary?): boolean
    return M.key(a) == M.key(b)
end

return M
