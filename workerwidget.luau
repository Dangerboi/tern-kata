--!strict

-- The kata worker widget as plain data: its id, name, icon, styles and the commands a click runs, and what it shows for a
-- summary. No Tern call is made here, so the rule is tested by `luau test/workerwidget_test.luau`. host.luau (header) says
-- which half draws it; both halves register the `spec` below.

local workersummary = require("./workersummary")
local widgets = require("./widgets")

type Summary = workersummary.Summary

local M = {}

M.ID = "worker"
M.NAME = "kata worker"

-- The ids of the window commands a click runs; window.luau registers the commands under these ids.
M.OPEN_ID = "open"
M.SHOW_HELD_ID = "show-held-runs"

-- The tooltip's lines are written so that what each line is for survives Tern's cut at LINE_CHARS (widgets.fit): the first line
-- is where the worker is, a held run is its hold (whose code comes first, "agent_quota: ...") and then its Issue, and the
-- executor, which can be a long host name, is last on the last line. The job is left out; the Issue names it. The host half's
-- widget shows the session host's worker in every window attached to it, while the window half's reads the worker of the
-- machine the window itself runs on, which on a remote window is not the session host.
local function title(summary: Summary, onHost: boolean): string
    local held: { string } = {}
    for _, run in summary.held do table.insert(held, run.reason .. " " .. run.ref) end
    local lines = {"Worker " .. (if onHost then "on the session host" else "on this machine")}
    for _, line in widgets.capped(held) do table.insert(lines, line) end
    table.insert(lines, widgets.fit(summary.running .. " running, process " .. summary.pid .. ", " .. summary.executor))
    return table.concat(lines, "\n")
end

local function view(summary: Summary?, onHost: boolean, plugin: string): WidgetView?
    if summary == nil then return nil end
    local value = if summary.capacity > 0 then summary.inUse / summary.capacity else 0
    local held = #summary.held
    if held > 0 then
        return {text = held .. " held", tone = "warning", title = title(summary, onHost), command = widgets.commandId(plugin, M.SHOW_HELD_ID), value = value}
    end
    local text = if summary.running > 0 then summary.running .. " running" else "idle"
    return {text = text, tone = "muted", title = title(summary, onHost), command = widgets.commandId(plugin, M.OPEN_ID), value = value}
end

-- What `tern.chrome.widget(M.ID, ...)` takes, for the half that draws the session host's worker (`onHost`) or this machine's;
-- `summary` is read at each render.
function M.spec(plugin: string, onHost: boolean, summary: () -> Summary?): WidgetSpec
    return {
        name = M.NAME,
        description = "What the kata worker on " .. (if onHost then "the session host" else "this machine") .. " is running, and the runs held for you; a click opens them",
        icon = "server",
        styles = {"text", "ring"} :: { WidgetStyle },
        render = function(_pane): WidgetView?
            return view(summary(), onHost, plugin)
        end,
    }
end

return M
