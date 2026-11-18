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
-- The ids of the buttons on a held run's card row; host.luau handles their `widget_action`, whose `row` is the run's uid.
M.RESUME_ID = "resume"
M.ABANDON_ID = "abandon"

-- Tern keeps a hover card's first 32 rows and leaves out a row whose label or text runs over 200 characters.
local CARD_ROWS = 32
local CARD_CHARS = 200
-- A card row's id, which a button needs, is 1 to 64 characters.
local ROW_ID_CHARS = 64
-- Tern draws each card row on one line and cuts it with an ellipsis. A row without a label spans the card (272 px of sans
-- text on 0.9.27, about 49 average characters), so a hold reason is wrapped by words at REASON_CHARS onto at most
-- REASON_LINES such rows. A job name, which a run's second row starts with, is cut at JOB_CHARS.
local REASON_CHARS = 44
local REASON_LINES = 3
local JOB_CHARS = 24

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

-- How long ago `since` was at `now` (Unix seconds), in the largest whole unit: "<1 m", "12 m", "3 h", "2 d".
function M.age(since: number, now: number): string
    local seconds = math.max(0, now - since)
    if seconds < 60 then return "<1 m" end
    if seconds < 3600 then return seconds // 60 .. " m" end
    if seconds < 86400 then return seconds // 3600 .. " h" end
    return seconds // 86400 .. " d"
end

-- The rows of one held run, as a heading, a byline and a body:
-- 1. its Issue, in the accent tone: the ref as the label and the Issue's title as the text, which opens the Issue (the ref is
--    the text when the worker gave no title, as a binary from before titles does);
-- 2. its job and how long it has been held, with a button: Resume when the worker would resume the run, and Abandon when it
--    would not but would abandon it, which is then the only way to settle it (as `worker status` says). That row has the
--    error tone with Abandon, which the stylesheet draws the button in, and the muted tone otherwise. Neither button is
--    offered while the worker would take neither verb (its hold is still being reported, or its agent's process is still
--    there), nor by the window half's widget, as only the session host's half runs the command;
-- 3. the worker's sentence for the hold, wrapped on full-width rows.
local function heldRows(run: workersummary.Held, onHost: boolean, now: number): { WidgetCardRow }
    local ref = if run.ref ~= "" then widgets.cut(run.ref, CARD_CHARS) else "no Issue"
    local titled = run.title ~= ""
    local head: WidgetCardRow = {
        label = if titled then ref else nil,
        text = if titled then widgets.cut(run.title, CARD_CHARS) else ref,
        href = issueLink(run.ref),
        tone = "accent",
    }
    local byline = if run.job ~= "" then widgets.cut(run.job, JOB_CHARS) else "held"
    if run.heldAt ~= nil then byline ..= " · " .. M.age(run.heldAt, now) end
    local meta: WidgetCardRow = {label = byline, text = "", tone = "muted"}
    if onHost and #run.uid >= 1 and #run.uid <= ROW_ID_CHARS then
        if run.resumable then
            meta.id = run.uid
            meta.actions = {{id = M.RESUME_ID, label = "Resume"}}
        elseif run.abandonable then
            meta.id = run.uid
            meta.tone = "error"
            meta.actions = {{id = M.ABANDON_ID, label = "Abandon"}}
        end
    end
    local rows = {head, meta}
    for _, line in widgets.wrap(run.reason, REASON_CHARS, REASON_LINES) do
        table.insert(rows, {text = line, tone = "warn"})
    end
    return rows
end

-- The hover card (Tern 0.9.27+; an older Tern shows `title`): each held run as heldRows lays it out, then what runs and which
-- worker this is: its executor and pid, and "this machine" on the window half's widget, which reads its own machine's worker
-- rather than the session host's. Held runs past Tern's row limit become one "and N more held" row.
local function card(summary: Summary, onHost: boolean, now: number): { WidgetCardRow }
    local rows: { WidgetCardRow } = {}
    -- The Running and Worker rows close the card.
    local limit = CARD_ROWS - 2
    for index, run in summary.held do
        local block = heldRows(run, onHost, now)
        local more = if index < #summary.held then 1 else 0
        if #rows + #block + more > limit then
            table.insert(rows, {text = "and " .. (#summary.held - index + 1) .. " more held", tone = "muted"})
            break
        end
        for _, row in block do table.insert(rows, row) end
    end
    table.insert(rows, {label = "Running", text = tostring(summary.running)})
    local names = {}
    if summary.executor ~= "" then table.insert(names, summary.executor) end
    if not onHost then table.insert(names, "this machine") end
    table.insert(names, "pid " .. summary.pid)
    table.insert(rows, {label = "Worker", text = widgets.cut(table.concat(names, " · "), CARD_CHARS)})
    return rows
end

local function view(summary: Summary?, onHost: boolean, plugin: string, now: number): WidgetView?
    if summary == nil then return nil end
    local value = if summary.capacity > 0 then summary.inUse / summary.capacity else 0
    local held = #summary.held
    if held > 0 then
        return {text = held .. " held", tone = "warning", title = title(summary, onHost), card = card(summary, onHost, now), command = widgets.commandId(plugin, M.SHOW_HELD_ID), value = value}
    end
    local text = if summary.running > 0 then summary.running .. " running" else "idle"
    return {text = text, tone = "muted", title = title(summary, onHost), card = card(summary, onHost, now), command = widgets.commandId(plugin, M.OPEN_ID), value = value}
end

-- What the widget shows changes with the clock while runs are held (their ages): a caller that redraws only when this key
-- changes, besides the summary itself, keeps the ages current to the minute.
function M.clockKey(summary: Summary?, now: number): string
    if summary == nil or #summary.held == 0 then return "" end
    return tostring(now // 60)
end

-- What `tern.chrome.widget(M.ID, ...)` takes, for the half that draws the session host's worker (`onHost`) or this machine's;
-- `summary` is read at each render, and `clock` (Unix seconds; os.time by default) dates the held runs.
function M.spec(plugin: string, onHost: boolean, summary: () -> Summary?, clock: (() -> number)?): WidgetSpec
    local now: () -> number = clock or function() return os.time() end
    return {
        name = M.NAME,
        description = "What the kata worker on " .. (if onHost then "the session host" else "this machine") .. " is running, and the runs held for you; a click opens them",
        icon = "server",
        styles = {"text", "ring"} :: { WidgetStyle },
        render = function(_pane): WidgetView?
            return view(summary(), onHost, plugin, now())
        end,
    }
end

-- The window's stylesheet for this widget's hover card, which Tern draws as `.wgc[data-widget="<plugin>/<id>"]` with one
-- `.wgc-plugin-row` per card row (`.bare` without a label, `data-tone` its tone). It gives the card the built-in Agents card's
-- hierarchy, where Tern draws every plugin row alike. A held run's heading (the accent row) is its Issue ref as a chip and its
-- title in bold, cut with an ellipsis; its byline (the row after the heading) is muted, with the button at its end; its
-- reason is a muted paragraph behind a rule in the warning colour. Runs are spaced apart, and Running and Worker sit under a
-- line as the card's footer. Resume is a solid pill in the theme's accent fill (with white text, as the Pane's selected rows
-- use it); Abandon, on an error-toned byline, an outlined pill in the error colour, so the button that ends a run does not
-- look like the one that continues it. Tern's own card buttons are plain text. A window without this plugin, or a Tern whose
-- card markup changed, draws the card plain, the heading in the accent colour.
function M.css(plugin: string): string
    local card = '.wgc[data-widget="' .. plugin .. "/" .. M.ID .. '"]'
    local head = card .. ' .wgc-plugin-row[data-tone="accent"]'
    local byline = head .. " + .wgc-plugin-row"
    local abandon = card .. ' .wgc-plugin-row[data-tone="error"] .wgc-plugin-act'
    return table.concat({
        head .. ":not(:first-child) {\n\tmargin-top: 10px;\n}",
        head .. " .wgc-k {\n\tflex: none;\n\talign-self: center;\n\tmargin-right: 2px;\n\tpadding: 0 5px;\n\tborder-radius: 4px;\n\tline-height: 18px;\n\tfont-family: var(--mono);\n\tfont-size: 11.5px;\n\tcolor: var(--t1);\n\tbackground-color: color-mix(in srgb, var(--t1) 8%, transparent);\n}",
        head .. " .wgc-v {\n\tflex: 1 1 auto;\n\tmin-width: 0;\n\toverflow: hidden;\n\ttext-overflow: ellipsis;\n\twhite-space: nowrap;\n\ttext-align: left;\n\tfont-family: inherit;\n\tfont-size: 13px;\n\tfont-weight: 600;\n\tcolor: var(--t1);\n\ttext-decoration: none;\n}",
        head .. " .wgc-plugin-link:hover {\n\tcolor: var(--accent);\n\ttext-decoration: underline;\n}",
        byline .. " {\n\tmin-height: 0;\n}",
        byline .. " .wgc-k {\n\tflex: 1 1 auto;\n\tmin-width: 0;\n\toverflow: hidden;\n\ttext-overflow: ellipsis;\n\twhite-space: nowrap;\n\tcolor: var(--t2);\n}",
        card .. ' .wgc-plugin-row.bare[data-tone="warn"] {\n\tmin-height: 0;\n\theight: 18px;\n\tmargin-left: 2px;\n\tpadding-left: 10px;\n\tborder-left: 2px solid var(--warn);\n}',
        card .. ' .wgc-plugin-row.bare[data-tone="warn"] .wgc-v {\n\tcolor: var(--t2);\n}',
        card .. " .wgc-plugin-row:nth-last-child(2) {\n\tmargin-top: 8px;\n\tpadding-top: 6px;\n\tborder-top: 1px solid color-mix(in srgb, var(--t1) 12%, transparent);\n}",
        card .. " .wgc-plugin-act {\n\tflex: none;\n\tmargin-left: auto;\n\tborder: 1px solid transparent;\n\tborder-radius: 999px;\n\tpadding: 0 10px;\n\tcolor: #fff;\n\tbackground-color: var(--accent-fill);\n}",
        card .. " .wgc-plugin-act:hover {\n\tbackground-color: color-mix(in srgb, var(--accent-fill) 85%, #000);\n}",
        abandon .. " {\n\tcolor: var(--bad);\n\tbackground-color: transparent;\n\tborder-color: color-mix(in srgb, var(--bad) 55%, transparent);\n}",
        abandon .. ":hover {\n\tbackground-color: color-mix(in srgb, var(--bad) 14%, transparent);\n}",
    }, "\n") .. "\n"
end

return M
