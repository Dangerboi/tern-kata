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
-- The id of the Resume button on a held run's card row; host.luau handles its `widget_action`, whose `row` is the run's uid.
M.RESUME_ID = "resume"

-- Tern keeps a hover card's first 32 rows and leaves out a row whose label or text runs over 200 characters.
local CARD_ROWS = 32
local CARD_CHARS = 200
-- A card row's id, which a button needs, is 1 to 64 characters.
local ROW_ID_CHARS = 64

-- The tooltip, which a Tern before 0.9.27 shows instead of the card (`card` below). Its lines are written so that what each
-- line is for survives Tern's cut at LINE_CHARS (widgets.fit): the first line is where the worker is, a held run is its hold
-- (whose code comes first, "agent_quota: ...") and then its Issue, and the
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

-- The Issue's link as tern-kata's link route (window.luau) opens it, for a ref `<project>#<short id>`; nil for a ref it cannot
-- carry, which would make Tern leave the row out.
local function issueLink(ref: string): string?
    local project, short = ref:match("^(.+)#(%w+)$")
    if project == nil or short == nil or project:find("[%s%c/?#%%\\]") then return nil end
    return "https://kata.invalid/" .. project .. "/" .. short
end

-- The hover card (Tern 0.9.27+; an older Tern shows `title`): where the worker is, a row per held run (its Issue, which a
-- click opens, and the worker's sentence for the hold, with a Resume button when the worker would resume it and the session
-- host's half draws the widget, as that half runs the command), then what runs and the process. Held runs past what fits
-- become one "and N more held" row.
local function card(summary: Summary, onHost: boolean): { WidgetCardRow }
    local rows: { WidgetCardRow } = {{label = "Worker", text = if onHost then "on the session host" else "on this machine", tone = "muted"}}
    local room = CARD_ROWS - 3
    if #summary.held > room then room -= 1 end
    for index, run in summary.held do
        if index > room then
            table.insert(rows, {text = "and " .. (#summary.held - room) .. " more held", tone = "muted"})
            break
        end
        local row: WidgetCardRow = {
            label = if run.ref ~= "" then widgets.cut(run.ref, CARD_CHARS) else nil,
            text = widgets.cut(if run.reason ~= "" then run.reason else "held", CARD_CHARS),
            tone = "warn",
            href = issueLink(run.ref),
        }
        if onHost and run.resumable and #run.uid >= 1 and #run.uid <= ROW_ID_CHARS then
            row.id = run.uid
            row.actions = {{id = M.RESUME_ID, label = "Resume"}}
        end
        table.insert(rows, row)
    end
    table.insert(rows, {label = "Running", text = tostring(summary.running)})
    local process = tostring(summary.pid)
    if summary.executor ~= "" then process ..= " on " .. summary.executor end
    table.insert(rows, {label = "Process", text = widgets.cut(process, CARD_CHARS)})
    return rows
end

local function view(summary: Summary?, onHost: boolean, plugin: string): WidgetView?
    if summary == nil then return nil end
    local value = if summary.capacity > 0 then summary.inUse / summary.capacity else 0
    local held = #summary.held
    if held > 0 then
        return {text = held .. " held", tone = "warning", title = title(summary, onHost), card = card(summary, onHost), command = widgets.commandId(plugin, M.SHOW_HELD_ID), value = value}
    end
    local text = if summary.running > 0 then summary.running .. " running" else "idle"
    return {text = text, tone = "muted", title = title(summary, onHost), card = card(summary, onHost), command = widgets.commandId(plugin, M.OPEN_ID), value = value}
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
