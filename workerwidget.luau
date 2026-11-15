--!strict

-- What the kata worker widget in the widget bar shows, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/workerwidget_test.luau`. host.luau asks `view` for what its worker widget draws from the summary it keeps; on a
-- Tern whose host half cannot offer widgets it stores the summary under the `tern.kv` key `workersummary.KEY` instead, and
-- window.luau reads it back and asks `view` for the window half's widget.

local workersummary = require("./workersummary")
local widgets = require("./widgets")

type Summary = workersummary.Summary

local M = {}

-- The ids of the window commands a click runs; window.luau registers the commands under these ids.
M.OPEN_ID = "open"
M.SHOW_HELD_ID = "show-held-runs"

function M.openCommand(plugin: string): string
    return "plugin." .. plugin .. "." .. M.OPEN_ID
end

function M.heldCommand(plugin: string): string
    return "plugin." .. plugin .. "." .. M.SHOW_HELD_ID
end

-- The executor and pid are named, and the tooltip says whose machine the worker is on: the widget the host half draws shows
-- the session host's worker in every window attached to it, while the window half's fallback widget reads the worker of the
-- machine the window itself runs on, which on a remote window is not the session host. Tern cuts each tooltip line at about
-- 45 characters, so the first line keeps only where the worker is, the pid goes on the last line, and a held run is its
-- Issue and then its hold, whose code comes first ("agent_quota: ..."); the job is left out, the Issue names it.
local function title(summary: Summary, onHost: boolean): string
    local held: { string } = {}
    for _, run in summary.held do table.insert(held, run.ref .. " " .. run.reason) end
    local where = if onHost then "on the session host" else "on this machine"
    local lines = {"Worker " .. summary.executor .. " " .. where}
    for _, line in widgets.capped(held) do table.insert(lines, line) end
    table.insert(lines, summary.running .. " running, process " .. summary.pid)
    return table.concat(lines, "\n")
end

function M.view(summary: Summary?, onHost: boolean, heldCommand: string, openCommand: string): WidgetView?
    if summary == nil then return nil end
    local value = if summary.capacity > 0 then summary.inUse / summary.capacity else 0
    local held = #summary.held
    if held > 0 then
        return {text = held .. " held", tone = "warning", title = title(summary, onHost), command = heldCommand, value = value}
    end
    local text = if summary.running > 0 then summary.running .. " running" else "idle"
    return {text = text, tone = "muted", title = title(summary, onHost), command = openCommand, value = value}
end

return M
