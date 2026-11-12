--!strict

-- What the kata worker widget in the widget bar shows, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/workerwidget_test.luau`. The host half stores a summary of the worker under the `tern.kv` key `workersummary.KEY`;
-- window.luau reads it on a timer, keeps it, and asks `view` for what to draw.

local workersummary = require("./workersummary")
local widgets = require("./widgets")

type Summary = workersummary.Summary

local M = {}

-- The tooltip: which worker this is, then each held run with why it waits (at most five, then how many more), then how many
-- runs go on. The executor and pid are named so a remote window never mistakes this machine's worker for the session host's.
local function title(summary: Summary): string
    local held: { string } = {}
    for _, run in summary.held do table.insert(held, run.ref .. " " .. run.job .. ": " .. run.reason) end
    local lines = {"Worker " .. summary.executor .. " on this machine (process " .. summary.pid .. ")"}
    for _, line in widgets.capped(held) do table.insert(lines, line) end
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

return M
