--!strict

local placement = require("./placement")
local releases = require("./releases")
local panecommands = require("./panecommands")
local waitingtext = require("./waitingtext")
local waitingagents = require("./waitingagents")
local workersummary = require("./workersummary")
local workerwidget = require("./workerwidget")
local widgets = require("./widgets")

-- kata Panes this plugin launched, until they close: each is reaped on its own, since every tab may have one.
local launched: { [number]: boolean } = {}
local fresh: { [number]: boolean } = {}
-- Issue reference per new kata Pane, sent over TSP once its surface is live, so it never passes through a shell.
local pending: { [number]: string? } = {}
-- The actions owed to a kata Pane still starting, in the order they were asked for: Wrap up and Show held runs can both
-- arrive before the Pane's surface is live, and neither may overwrite the other.
local pendingActions: { [number]: { { act: string, value: string } } } = {}
local function owe(pane: number, act: string, value: string)
    local list = pendingActions[pane] or {}
    pendingActions[pane] = list
    for _, owed in list do
        if owed.act == act and owed.value == value then return end
    end
    table.insert(list, {act = act, value = value})
end
-- The Panes `Update tern-kata` launched: they run the tern-kata program but are not a kata to link or mark.
-- Kept in `tern.kv` too, because a plugin reload (a local update brings the plugin along) empties this table while the
-- Pane is still open; `pane_closed` removes the id from both.
local KV_UPDATE_PANES = "update.panes"
local UPDATE_PANE_TTL = 24 * 60 * 60
local updatePanes: { [number]: boolean } = {}

-- `tern.kv` is shared by every window, so the key is read, changed by one id and written back in one step rather than
-- written from this VM's copy. Entries carry the time they were added; one older than a day is dropped, so an id left
-- behind by a Pane that ended while no window was open cannot outlive a later Pane that reuses it.
local function readUpdatePanes(): { { pane: number, at: number } }
    local entries: { { pane: number, at: number } } = {}
    local ok, saved = pcall(tern.kv.get, KV_UPDATE_PANES)
    if not ok or type(saved) ~= "table" then return entries end
    local now = os.time()
    for _, e in saved :: { any } do
        if type(e) == "table" and type(e.pane) == "number" and type(e.at) == "number" and now - e.at < UPDATE_PANE_TTL then
            table.insert(entries, {pane = e.pane, at = e.at})
        end
    end
    return entries
end

local function changeUpdatePanes(id: number, add: boolean)
    local entries = readUpdatePanes()
    for i = #entries, 1, -1 do
        if entries[i].pane == id then table.remove(entries, i) end
    end
    if add then table.insert(entries, {pane = id, at = os.time()}) end
    tern.kv.set(KV_UPDATE_PANES, if #entries > 0 then entries else nil)
    if add then updatePanes[id] = true else updatePanes[id] = nil end
end

for _, e in readUpdatePanes() do updatePanes[e.pane] = true end
-- A kata Pane has the title `tern-kata` (which the binary sets), or a program whose file name is tern-kata or
-- tern-kata.exe, split on either path separator so Windows paths match. A longer name merely containing it does not.
local function isKataProgram(program: string?): boolean
    if program == nil then return false end
    local name = (program:match("[^/\\]*$") or ""):lower()
    return name == "tern-kata" or name == "tern-kata.exe"
end

local function isKata(info: PaneInfo?): boolean
    return info ~= nil and info.exited == nil and not updatePanes[info.pane]
        and (info.title == "tern-kata" or isKataProgram(info.program))
end

-- kata's mark, the katakana ka from kata's favicon, replaces the terminal icon on kata Panes' tab faces. Tern's program icons
-- take only its built-in names, and a terminal running tern-kata has no program key for a `data-program` rule, so the
-- stylesheet names each kata Pane by id. The mark is drawn black and tinted to the face's colour (dim, focused or a
-- state's) by Tern's monochrome filter, as Tern's own glyph faces are.
local MARK = "url(\"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='1.25 2.5 26 26' fill='none' "
    .. "stroke='black' stroke-width='2.4' stroke-linecap='round' stroke-linejoin='round'>"
    .. "<path d='M6.5 11.5 H22.5 V20.5 Q22.5 25 18 25.5'/><path d='M13.5 5.5 C13.5 14 10.5 20.5 6 25.5'/></svg>\")"
local marked: { [number]: boolean } = {}
local markCss = ""

-- A tab face's `data-pane` holds the pane's full id, but Lua gets a remote host's pane ids packed below 2^53 (a double holds no
-- more): bit 52 set, the host slot in bits 44-51, the id's bits 32-47 in bits 24-39 and its bits 0-23 in bits 0-23 (the
-- id's bits 24-31 are always zero, which is what makes the packing fit). The face id is host * 2^56 + that id, above 2^53
-- itself, so it is built as decimal text. A local id (below 2^52) is the face's own.
local function faceId(pane: number): string
    if pane < 2 ^ 52 then return string.format("%d", pane) end
    local host = math.floor(pane / 2 ^ 44) % 256
    local low = pane % 2 ^ 44
    -- host * 2^56 + bits 32-47 * 2^32 + bits 0-23 = (host * 2^24 + bits 32-47) * 2^32 + bits 0-23, the first factor under 2^32:
    -- doubled 32 times in base-10^7 digits (least significant first), then the low bits added.
    local top = host * 2 ^ 24 + math.floor(low / 2 ^ 24)
    local digits: { number } = {}
    while top > 0 do
        table.insert(digits, top % 1e7)
        top = math.floor(top / 1e7)
    end
    for _ = 1, 32 do
        local carry = 0
        for i, digit in digits do
            local doubled = digit * 2 + carry
            digits[i] = doubled % 1e7
            carry = doubled // 1e7
        end
        if carry > 0 then table.insert(digits, carry) end
    end
    local carry = low % 2 ^ 24
    for i, digit in digits do
        local sum = digit + carry
        digits[i] = sum % 1e7
        carry = sum // 1e7
    end
    if carry > 0 then table.insert(digits, carry) end
    local text = string.format("%d", digits[#digits])
    for i = #digits - 1, 1, -1 do text ..= string.format("%07d", digits[i]) end
    return text
end

local function restyleMarks()
    local faces: { string } = {}
    for pane in marked do table.insert(faces, string.format(".tn-face[data-pane='%s']", faceId(pane))) end
    table.sort(faces)
    local css = ""
    if #faces > 0 then
        local children: { string } = {}
        for i, face in faces do children[i] = face .. " > *" end
        css = table.concat(children, ",\n") .. " {\n\tdisplay: none;\n}\n"
            .. table.concat(faces, ",\n") .. " {\n\tbackground-image: " .. MARK .. ";\n"
            .. "\tbackground-size: contain;\n\tbackground-repeat: no-repeat;\n\tbackground-position: center;\n"
            .. "\tfilter: monochrome(currentColor 1 1px);\n}\n"
    end
    if css ~= markCss then
        markCss = css
        tern.css("mark", css)
    end
end

local function markIf(cx: WindowCx, pane: number)
    local kata = isKata(cx.session:pane(pane))
    if kata == (marked[pane] == true) then return end
    if kata then marked[pane] = true else marked[pane] = nil end
    restyleMarks()
end

-- Links reuse a kata Pane only in the focused Pane's tab; a parked Pane has no tab.
local function existingPane(cx: WindowCx): number?
    local focused = cx.session:focused()
    local tab = focused and cx.session:tab_of(focused)
    if not tab then return nil end
    for _, info in cx.session:panes() do
        if info.tab == tab and isKata(info) then return info.pane end
    end
    return nil
end

-- An action the kata Pane's issue list takes over TSP, so a value never passes through a shell.
local function paneAction(cx: WindowCx, pane: number, act: string, value: string)
    cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = act, value = value})
end

local relayed: { [number]: boolean } = {}
local retrying: { [number]: boolean } = {}

-- Background agents live in a session of this name (internal/model AgentSession), kept out of sight.
local AGENT_SESSION = "kata agents"

local function hiddenTabs(cx: WindowCx, all: { TabInfo }): { [number]: boolean }
    local sessions: { [number]: boolean } = {}
    for _, session in cx.sessions:list() do
        if session.name == AGENT_SESSION then sessions[session.id] = true end
    end
    local tabs: { [number]: boolean } = {}
    for _, tab in all do
        if sessions[tab.session] then tabs[tab.id] = true end
    end
    return tabs
end

local function waitingMessage(cx: WindowCx, agent: AgentBlock, last: string): string
    local ok, transcript = pcall(function() return cx.agents:transcript(agent.pane, {last = 2}) end)
    return waitingtext.of(if ok then transcript else {}, last)
end

-- Every agent block in the session as JSON for the Pane, which matches it against its own agents. The pane id goes as
-- decimal text; %.0f prints a whole number without an exponent. Tern's API gives the id only as a Luau number (a double),
-- so the text is exact only for ids up to 2^53: a larger id is already rounded here and matches no agent of the Pane.
-- A window on another machine numbers panes differently from the session host, so the id may match nothing there; the command
-- (it names the agent's prompt file) lets the Pane find its agent anyway. A missing last message or command is the empty string.
-- The last message of an agent waiting for you is what it waits on (waitingtext.luau), read from its transcript: Tern's last
-- message can be a cut-off thinking block. The transcript is read only for a waiting agent, and only its end.
-- `hidden` says whether the agent's tab is in the hidden session, so the Pane learns when an agent is opened or put away
-- without it; it is left out when the window cannot place the agent's tab.
local function agentStates(cx: WindowCx, tabs: { TabInfo }): string
    local list = tern.json.array({})
    local hidden = hiddenTabs(cx, tabs)
    for _, agent in cx.agents:list() do
        local last = if type(agent.last_message) == "string" then agent.last_message else ""
        if agent.state == "waiting_input" then last = waitingMessage(cx, agent, last) end
        local tab = cx.session:tab_of(agent.pane)
        table.insert(list, {
            pane = string.format("%.0f", agent.pane),
            state = agent.state,
            last_message = last,
            command = if type(agent.command) == "string" then agent.command else "",
            hidden = if tab then hidden[tab] == true else nil,
        })
    end
    return tern.json.encode(list)
end

local function relayStates(cx: WindowCx, pane: number, states: string)
    if not cx.session:surface(pane) then return end
    relayed[pane] = true
    paneAction(cx, pane, "agent-states", states)
end

-- Places new agent tabs; defined with the tab placement below, and run on every relay pass.
local placeAgents: (WindowCx, { TabInfo }, { PaneInfo }) -> ()

local function relayAll(cx: WindowCx, onlyNew: boolean)
    local tabs = cx.session:tabs()
    local panes = cx.session:panes()
    placeAgents(cx, tabs, panes)
    local states: string? = nil
    for _, info in panes do
        if isKata(info) and not (onlyNew and relayed[info.pane]) and cx.session:surface(info.pane) then
            if not states then states = agentStates(cx, tabs) end
            relayStates(cx, info.pane, states)
        end
    end
end

local function showSessionOf(cx: WindowCx, tab: number?)
    for _, info in cx.session:tabs() do
        if info.id == tab then cx.sessions:switch(info.session) end
    end
end

local function deliver(cx: WindowCx, pane: number)
    local ref = pending[pane]
    if ref and cx.session:surface(pane) then
        pending[pane] = nil
        paneAction(cx, pane, "select-issue", ref)
    end
    local owed = pendingActions[pane]
    if owed and cx.session:surface(pane) then
        pendingActions[pane] = nil
        for _, item in owed do paneAction(cx, pane, item.act, item.value) end
    end
    if not relayed[pane] then relayAll(cx, true) end
end

-- Startup-failure rule, the plugin's half of the contract with cmd/tern-kata: a kata Pane failing within 2 s with
-- status 2 (a Go panic, or a command line tern-kata refused) or 3 (the binary's own error) means tern-kata ran and failed. Status 1 can be either: it is
-- PowerShell's not-found code, and on Windows Tern reports every failing launch as 1, so tern-kata's 3 never arrives.
-- Any other failing status is the shell's own not-found code (127, 9009), so it is not installed or not on PATH.
local function exitedRightAway(cx: WindowCx, pane: number, status: number)
    fresh[pane] = nil
    if status == 0 then return end
    if status == 2 or status == 3 then
        cx:toast("error", "tern-kata failed to start", "tern-kata may be older than this plugin: run Update tern-kata from the command palette, or run tern-kata in a shell to see why")
    elseif status == 1 then
        cx:toast("error", "tern-kata didn't start", "It may be older than this plugin: run Update tern-kata; if it isn't installed, run Install tern-kata. Or run tern-kata in a shell to see why")
    else
        cx:toast("error", "tern-kata is not installed or not on PATH", "Run Install tern-kata from the command palette, or the install script in the README")
    end
end

-- After the window, a failing exit still closes the Pane (spec), silently: poll once a second until it is gone.
local function reap(pane: number)
    tern.timer(1000, function(cx)
        if not cx or not launched[pane] then return end
        local info = cx.session:pane(pane)
        if not info then return end
        if info.exited ~= nil then cx.layout:close(pane); return end
        deliver(cx, pane)
        reap(pane)
    end)
end

-- A kata Pane exited with a failure within the first 2 s (20 ticks of 100 ms) means the binary is missing or failed at
-- startup. Tern closes the Pane on a clean exit (a quick quit is fine) but keeps it, marked exited, on a failing one,
-- so close it here.
local function watch(pane: number, ticks: number)
    fresh[pane] = true
    tern.timer(100, function(cx)
        if not cx or not fresh[pane] then return end
        local info = cx.session:pane(pane)
        if not info then fresh[pane] = nil; return end
        if info.exited ~= nil then
            exitedRightAway(cx, pane, info.exited)
            cx.layout:close(pane)
            return
        end
        deliver(cx, pane)
        if ticks > 1 then watch(pane, ticks - 1) else fresh[pane] = nil; reap(pane) end
    end)
end

-- A new Pane running `command`: a split of the focused Pane, so it starts on the session host, or a tab when none is focused.
local function launch(cx: WindowCx, command: string): number?
    local target = cx.session:focused()
    if target then return cx.layout:split(target, "right", {command = command}) end
    return cx.layout:new_tab({command = command})
end

-- The focused tab's kata Pane (when `find`) gets focus and `act`; otherwise a new Pane is launched, `queue` records what it does once
-- its surface is up, and it is watched for a failing start.
local function openKata(cx: WindowCx, find: boolean, act: (number) -> (), queue: (number) -> ())
    if find then
        local pane = existingPane(cx)
        if pane then
            cx.layout:focus(pane)
            act(pane)
            return
        end
    end
    local pane = launch(cx, panecommands.KATA)
    if not pane then return end
    launched[pane] = true
    queue(pane)
    watch(pane, 20)
end

local function open(cx: WindowCx, ref: string?)
    openKata(cx, ref ~= nil, function(pane) paneAction(cx, pane, "select-issue", ref :: string) end, function(pane) pending[pane] = ref end)
end

-- `W` starts an agent in a new tab, which Tern appends as the last tab. The agent's prompt file is named
-- `prompt-<starter>-<random>.md` (internal/data), `<starter>` being the block of the starting kata Pane, and that name is in the
-- command line Tern reports for the new tab's Pane: the agent block's `command`, else the running line. The tab is moved right of the
-- starting Pane's tab; no other tab is reordered.
-- Two ways find a new agent tab. `tab_created` fires for tabs opened on this window's own machine. A window on another machine
-- gets no `tab_created` for a tab the CLI opens on the session host, and numbers panes differently from the host, so
-- `<starter>` is no pane id there. Such a window places on `agent_state` and on every relay pass instead: an agent pane whose
-- command carries the marker is tried once, whatever the result (remembered in `placed` until its pane closes), and then never
-- touched again. The starter id is exact only on the session host's machine, so the move is a guess there. In its session the
-- tab moves only when the run of tabs at the end that are candidates (agents tried in this pass) or agent tabs not handled yet
-- (a pane not in `placed` whose running line carries the marker, even if Tern lists no agent for it yet) contains it, and
-- the tab directly before that run holds no kata Pane: Tern's append already sits right of a kata Pane, possibly placed
-- exactly by the host window. A tab after it that is neither, or a kata Pane's tab just before the run, leaves the tab
-- where it is. So with no window on the session host, `W` in a kata Pane while another kata Pane's tab is
-- last does not move the agent tab. The host window's exact placement is the `tab_created` one.
-- The starter is the kata Pane whose id is `<starter>`, else the focused Pane if it is a kata Pane (the Pane the user just pressed
-- `W` in), else the window's only kata Pane; with none of these nothing moves.
local PLACE_TRIES = 10
local placed: { [number]: boolean } = {}

-- An agent block's unset command is tern.json.null, not nil, so both readers check for a string.
local function markerOf(text: string?): number?
    local block = if type(text) == "string" then text:match("prompt%-(%d+)%-%w+%.md") else nil
    return block and tonumber(block) or nil
end

local function starterOf(cx: WindowCx, tab: number): number?
    for _, info in cx.session:panes() do
        if info.tab == tab and info.running then
            local block = markerOf(info.running.line)
            if block then return block end
        end
    end
    for _, agent in cx.agents:list() do
        local info = cx.session:pane(agent.pane)
        if info and info.tab == tab then
            local block = markerOf(agent.command)
            if block then return block end
        end
    end
    return nil
end

local function tabRecs(tabs: { TabInfo }): { placement.TabRec }
    local recs: { placement.TabRec } = {}
    for i, info in tabs do recs[i] = {id = info.id, session = info.session, position = info.position} end
    return recs
end

local function paneRecs(panes: { PaneInfo }): { placement.PaneRec }
    local recs: { placement.PaneRec } = {}
    for i, info in panes do
        recs[i] = {pane = info.pane, tab = info.tab, kata = isKata(info), marker = markerOf(info.running and info.running.line)}
    end
    return recs
end

local function placeTab(cx: WindowCx, tab: number, homeTab: number?)
    if not homeTab then return end
    local starterTab: TabInfo? = nil
    local newTab: TabInfo? = nil
    for _, info in cx.session:tabs() do
        if info.id == homeTab then starterTab = info end
        if info.id == tab then newTab = info end
    end
    if not starterTab or not newTab then return end
    local position = placement.positionBeside(starterTab, newTab)
    if position then cx.layout:order_tab(tab, position) end
end

local function placeAgentTab(cx: WindowCx, tab: number, tries: number)
    local starter = starterOf(cx, tab)
    if not starter then
        if tries > 1 then
            tern.timer(200, function(cx2)
                if cx2 then placeAgentTab(cx2, tab, tries - 1) end
            end)
        end
        return
    end
    -- A background agent's tab is in the hidden session and stays where it is.
    if not hiddenTabs(cx, cx.session:tabs())[tab] then
        -- Here the starter's own id is exact, since the tab is on this window's machine; a starter that is gone leaves the tab last.
        local home = cx.session:pane(starter)
        placeTab(cx, tab, home and home.tab)
    end
    for _, info in cx.session:panes() do
        if info.tab == tab then placed[info.pane] = true end
    end
end

-- Agent panes not placed yet whose command carries the marker, each tried once; where their tabs go is `placement.plan`, from
-- one snapshot of the window's tabs and panes taken by the relay pass. A background agent's tab is in the hidden session, which
-- is not ordered by the Pane that started it, so such an agent is never placed. The host window's exact placement is
-- `placeAgentTab`.
placeAgents = function(cx: WindowCx, tabs: { TabInfo }, panes: { PaneInfo })
    local agents: { placement.AgentRec } = {}
    for _, agent in cx.agents:list() do
        table.insert(agents, {pane = agent.pane, marker = markerOf(agent.command)})
    end
    local plan = placement.plan({
        tabs = tabRecs(tabs),
        panes = paneRecs(panes),
        agents = agents,
        hidden = hiddenTabs(cx, tabs),
        placed = placed,
        focused = cx.session:focused(),
    })
    for _, pane in plan.tried do placed[pane] = true end
    for _, move in plan.moves do cx.layout:order_tab(move.tab, move.position) end
end

-- Tern's notification for an agent that waits for the user focuses the agent where it lives, and a background agent lives in
-- the hidden session, which is meant to stay out of sight: the user would land in a session of nothing but agents. So focus
-- in that session opens the agent beside a kata Pane, as a click on its Agent badge does, and shows that Pane's session. The
-- kata Pane is the one the agent's prompt file names (this window's own Pane, when the id is exact), else the one this window
-- focused last, else its only one; with none the agent stays. The move leaves the hidden session, so the focus it raises
-- finds nothing to open. The relay that follows tells the Pane at once, so it records the agent as open.
local lastKata: number? = nil

local function isPromptCommand(command: string?): boolean
    return type(command) == "string" and command:find("agent%-prompts[/\\]prompt%-") ~= nil
end

local function openFocusedAgent(cx: WindowCx, pane: number, tab: number)
    if not hiddenTabs(cx, cx.session:tabs())[tab] then return end
    local command: string? = nil
    for _, agent in cx.agents:list() do
        if agent.pane == pane then command = agent.command end
    end
    if not isPromptCommand(command) then return end
    local kata = placement.kataPaneFor(paneRecs(cx.session:panes()), markerOf(command), lastKata)
    if not kata or not kata.tab then return end
    if not cx.layout:move(pane, kata.pane, "right") then return end
    showSessionOf(cx, kata.tab)
    cx.layout:focus(pane)
    relayAll(cx, false)
end

tern.on("focus", function(ev, cx)
    if isKata(cx.session:pane(ev.pane)) then
        lastKata = ev.pane
        return
    end
    openFocusedAgent(cx, ev.pane, ev.tab)
end)

-- The kata agents that wait for you. `waitOrder` numbers each agent's move into waiting (a later move, a larger number); it
-- is empty after a plugin reload, which leaves the agents already waiting last, in Tern's order. `waitingCount` is what the
-- "N waiting" segment draws in the widget bar's Plugin segments widget, and with `waitingTooltip` (the waiting agents' block
-- titles) what the Waiting kata agents widget draws: the formatters only read them, and they are refreshed when either
-- changes. Place the widget with the Add kata widgets to the widget bar command, or from the palette's Edit widgets....
local waitOrder: { [number]: number } = {}
local waitStamp = 0
local waitingCount = 0
local waitingTooltip = ""
local OPEN_ID = "open"
local GO_WAITING_ID = "go-to-waiting-agent"
local function commandName(id: string): string
    return "plugin.tern-kata." .. id
end
local GO_WAITING_COMMAND = commandName(GO_WAITING_ID)

local function waitingPanes(cx: WindowCx, skip: number?): { number }
    local agents: { waitingagents.Agent } = {}
    for _, agent in cx.agents:list() do
        table.insert(agents, {pane = agent.pane, command = agent.command, state = agent.state :: string})
    end
    return waitingagents.waiting(agents, waitOrder, isPromptCommand, skip)
end

local function recountWaiting(cx: WindowCx, skip: number?)
    local panes = waitingPanes(cx, skip)
    local titles: { string } = {}
    for _, pane in panes do
        local info = cx.session:pane(pane)
        table.insert(titles, if info ~= nil and info.title ~= "" then info.title else "Pane " .. pane)
    end
    local tooltip = table.concat(widgets.capped(titles), "\n")
    if #panes == waitingCount and tooltip == waitingTooltip then return end
    waitingCount = #panes
    waitingTooltip = tooltip
    tern.chrome.refresh()
end

tern.chrome.status(function(_pane)
    return waitingagents.segment(waitingCount, GO_WAITING_COMMAND)
end)

-- Tern before 0.9.17 has no widget bar, so no `tern.chrome.widget`: the widgets are skipped there with one log line and the rest
-- of the window half loads as before. `widgetsOffered` lists what registered, which the command that places them reads.
local widgetsOffered: { { id: string, name: string } } = {}
local widgetsSkipLogged = false
local function registerWidget(id: string, spec: WidgetSpec): boolean
    if tern.chrome.widget == nil then
        if not widgetsSkipLogged then
            widgetsSkipLogged = true
            tern.log.info("tern-kata: this Tern has no widget bar (before 0.9.17), so the tern-kata widgets are not offered")
        end
        return false
    end
    local ok, err = pcall(tern.chrome.widget, id, spec)
    if not ok then
        tern.log.warn("tern-kata: could not register widget " .. id .. ": " .. tostring(err))
        return false
    end
    table.insert(widgetsOffered, {id = id, name = spec.name})
    return true
end

registerWidget("waiting", {
    name = "Waiting kata agents",
    description = "How many agents kata started wait for you; a click goes to the one that began waiting last",
    icon = "bell",
    styles = {"text"},
    render = function(_pane)
        return waitingagents.widget(waitingCount, waitingTooltip, GO_WAITING_COMMAND)
    end,
})

-- The worker on this machine, as the host half's poll stores it. `tern.kv` here is this machine's, so a remote window shows its
-- own worker, never the session host's. The timer reads the key and keeps the answer; `render` only reads what was kept.
local WORKER_POLL_SECONDS = 5
local SHOW_HELD_ID = "show-held-runs"
-- The Page that lists the runs held for the user.
local HELD_PAGE = "attention"
local workerSummary: workersummary.Summary? = nil
-- What `workerSummary` was read as, to tell whether the next read differs.
local workerSummaryText = ""

local function pollWorkerSummary()
    tern.timer(WORKER_POLL_SECONDS * 1000, function(cx)
        if cx then
            local ok, saved = pcall(tern.kv.get, workersummary.KEY)
            local summary = if ok and type(saved) == "table" then workersummary.fresh(saved :: workersummary.Summary, os.time()) else nil
            local text = if summary ~= nil then tern.json.encode(summary) else ""
            if text ~= workerSummaryText then
                workerSummary = summary
                workerSummaryText = text
                tern.chrome.refresh()
            end
        end
        pollWorkerSummary()
    end)
end

if registerWidget("worker", {
    name = "kata worker",
    description = "What the kata worker on this machine is running, and the runs held for you; a click opens them",
    icon = "server",
    styles = {"text", "ring"},
    render = function(_pane)
        return workerwidget.view(workerSummary, commandName(SHOW_HELD_ID), commandName(OPEN_ID))
    end,
}) then
    pollWorkerSummary()
    tern.command({
        id = SHOW_HELD_ID,
        title = "Show kata's held runs",
        run = function(cx)
            openKata(cx, true, function(pane) paneAction(cx, pane, "show-page", HELD_PAGE) end, function(pane) owe(pane, "show-page", HELD_PAGE) end)
        end,
    })
end

if #widgetsOffered > 0 then
    tern.command({
        id = "add-widgets",
        title = "Add kata widgets to the widget bar",
        run = function(cx)
            local list = cx.settings:get("widgets")
            local ids: { string } = {}
            local names: { [string]: string } = {}
            for _, widget in widgetsOffered do
                table.insert(ids, widget.id)
                names[widget.id] = widget.name
            end
            local merged, added = widgets.merge(if type(list) == "table" then list else {}, ids)
            if #added == 0 then
                cx:toast("info", "The kata widgets are already in the widget bar")
                return
            end
            cx.settings:set("widgets", merged)
            local addedNames: { string } = {}
            for _, id in added do table.insert(addedNames, names[id]) end
            cx:toast("info", "Added " .. table.concat(addedNames, " and ") .. " to the widget bar")
        end,
    })
end

-- Tern's `cx.layout:focus` does not say that it raises the `focus` event, so the agent is opened here by the handler's own path
-- (a no-op for an agent outside the hidden session) before it is focused. A parked agent is dealt into the current tab first;
-- the agent's session is shown before the focus, as the focus handler's path does, so an agent in another session is reached.
tern.command({
    id = GO_WAITING_ID,
    title = "Go to the agent waiting for you",
    keys = {"shift+f9"},
    run = function(cx)
        local pane = waitingPanes(cx, nil)[1]
        local info = if pane then cx.session:pane(pane) else nil
        if not pane or not info then
            cx:toast("info", "No agent is waiting for you")
            return
        end
        if info.parked or not info.tab then
            local ok, err = cx.layout:unpark(pane)
            if not ok then
                cx:toast("error", err or "Could not bring the waiting agent back")
                return
            end
            info = cx.session:pane(pane)
        end
        local shown = if info then info.tab else nil
        if not shown then return end
        openFocusedAgent(cx, pane, shown)
        local now = cx.session:pane(pane)
        local tab = if now then now.tab else nil
        showSessionOf(cx, tab)
        cx.layout:focus(pane)
    end,
})

tern.on("tab_created", function(ev, cx)
    placeAgentTab(cx, ev.tab, PLACE_TRIES)
end)

tern.on("pane_closed", function(ev, cx)
    launched[ev.pane] = nil
    if updatePanes[ev.pane] then
        changeUpdatePanes(ev.pane, false)
    end
    pending[ev.pane] = nil
    pendingActions[ev.pane] = nil
    waitOrder[ev.pane] = nil
    recountWaiting(cx, ev.pane)
    relayed[ev.pane] = nil
    retrying[ev.pane] = nil
    placed[ev.pane] = nil
    if lastKata == ev.pane then lastKata = nil end
    if marked[ev.pane] then
        marked[ev.pane] = nil
        restyleMarks()
    end
    -- An agent that quits cleanly closes its block without an agent_state event; the relay that follows leaves the block out, and
    -- the kata Panes drop its entry when Tern no longer lists it.
    tern.timer(200, function(cx2)
        if cx2 then relayAll(cx2, false) end
    end)
end)

local function markAll(cx: WindowCx)
    for _, info in cx.session:panes() do
        if isKata(info) then marked[info.pane] = true end
    end
    restyleMarks()
    -- Agents already there were placed (or put somewhere on purpose) before this run: only later ones are moved.
    for _, agent in cx.agents:list() do placed[agent.pane] = true end
    recountWaiting(cx, nil)
    relayAll(cx, false)
end

-- The kata Panes already open: at window start, and after a plugin reload, which runs this file again but not window_start.
tern.on("window_start", markAll)
tern.timer(1, function(cx)
    if cx then markAll(cx) end
end)

-- tern-kata sets its title as it starts, so a title event finds a kata Pane started from any shell; other titles, such as an
-- agent's spinner, cost no lookup. The title is also set before the surface opens and again on every run in the same pane, so a
-- new one clears the sent mark, and a Pane without a surface is retried until it has one (10 s at most).
local RELAY_TRIES = 50

local function relayWhenLive(pane: number, tries: number)
    retrying[pane] = true
    tern.timer(200, function(cx)
        if not cx or not marked[pane] or relayed[pane] then retrying[pane] = nil; return end
        if cx.session:surface(pane) then
            retrying[pane] = nil
            relayAll(cx, true)
        elseif tries > 1 then
            relayWhenLive(pane, tries - 1)
        else
            retrying[pane] = nil
        end
    end)
end

tern.on("title", function(ev, cx)
    if ev.title == "tern-kata" or marked[ev.pane] then
        markIf(cx, ev.pane)
        if not marked[ev.pane] then return end
        if ev.title == "tern-kata" then relayed[ev.pane] = nil end
        if relayed[ev.pane] then return end
        relayAll(cx, true)
        if not relayed[ev.pane] and not retrying[ev.pane] then relayWhenLive(ev.pane, RELAY_TRIES) end
    end
end)

tern.on("agent_state", function(ev, cx)
    if ev.new == "waiting_input" then
        if ev.old ~= "waiting_input" then
            waitStamp += 1
            waitOrder[ev.pane] = waitStamp
        end
    else
        waitOrder[ev.pane] = nil
    end
    recountWaiting(cx, nil)
    relayAll(cx, false)
end)

-- The release check: window start, then every 24 hours while the window stays open. The decisions are in releases.luau; this
-- reads the clock, the environment, the store and the network. A failed fetch is a check like any other and stays silent.
local KV_LAST_CHECK = "release_check.last"
local KV_TOASTED = "release_check.toasted"
local KV_OFF = "release_check.off"

local function envOff(): boolean
    local value = tern.getenv("TERN_KATA_NO_UPDATE_CHECK")
    return value ~= nil and value ~= ""
end

local function ownVersion(): string?
    local ok, toml = pcall(tern.fs.read, tern.plugin.dir .. "/plugin.toml")
    if not ok or type(toml) ~= "string" then return nil end
    return releases.pluginVersion(toml)
end

-- `tern plugin dir`, asked once per load. A failed or raising ask (iOS) leaves nil, which counts the copy as installed.
local pluginDirAsked = false
local pluginDir: string? = nil
local pluginDirWaiting: { () -> () } = {}

local function withPluginDir(fn: () -> ())
    if pluginDirAsked then fn(); return end
    table.insert(pluginDirWaiting, fn)
    if #pluginDirWaiting > 1 then return end
    local function answer(dir: string?)
        pluginDirAsked = true
        pluginDir = dir
        local waiting = pluginDirWaiting
        pluginDirWaiting = {}
        for _, waiter in waiting do waiter() end
    end
    local ok = pcall(function()
        tern.process.run({"tern", "plugin", "dir"}, {timeout_ms = 10000}, function(result)
            local dir = result.status == 0 and result.stdout:match("^%s*(.-)%s*$") or nil
            answer(dir ~= "" and dir or nil)
        end)
    end)
    if not ok then answer(nil) end
end

local function versionTextUrl(): string
    local base = releases.releaseBase(tern.getenv("TERN_KATA_BASE_URL"), nil)
    return (base:gsub("/+$", "")) .. "/version.txt"
end

local function currentCheck(): releases.Check
    return {
        envOff = envOff(),
        linked = releases.isLinked(tern.plugin.dir, pluginDir),
        toggleOff = tern.kv.get(KV_OFF) == true,
        now = os.time(),
        lastCheck = (function(): number?
            local last = tern.kv.get(KV_LAST_CHECK)
            return type(last) == "number" and last or nil
        end)(),
    }
end

local function releaseCheck()
    withPluginDir(function()
        local check = currentCheck()
        if releases.shouldCheck(check) then
            -- Stored before the fetch: a failure counts as a check, and so does a second window starting meanwhile.
            tern.kv.set(KV_LAST_CHECK, check.now)
            tern.fetch(versionTextUrl(), {timeout_ms = 15000}, function(result, cx2)
                if not cx2 or result.error ~= nil or result.status ~= 200 then return end
                local own = ownVersion()
                if not own then return end
                local text, version = releases.toast(result.body, own, tern.kv.get(KV_TOASTED) :: string?)
                if text and version then
                    tern.kv.set(KV_TOASTED, version)
                    cx2:toast("info", text)
                end
            end)
        end
        -- While the window stays open the check comes round again; it fetches only when a day has passed since the last one.
        -- The variable and a linked plugin hold for the whole load; only the toggle can change meanwhile.
        local off = releases.offBecause(check)
        if off == nil or off == "toggle" then tern.timer(releases.DAY * 1000, releaseCheck) end
    end)
end

local function lastStderrLine(text: string): string
    local line = ""
    for l in text:gmatch("[^\r\n]+") do line = l end
    return line
end

-- The window machine's plugin step of Update tern-kata leaves its pending key behind; the plugin that loads after a
-- successful install is the version it names. A key of another version waits (the install may still be running) until
-- it is 10 minutes old.
local KV_PENDING = "update.pending"

-- Run once per load of this file: at window start, and after a plugin reload, which runs the file again without
-- window_start (the plugin the install left behind reports its update and carries the daily check on).
local loaded = false

-- Whether this Tern delivers a step that outlived its VM as an event (set where the handler is registered, below).
local hasProcessExit = false

local function onLoad(cx: WindowCx)
    if loaded then return end
    loaded = true
    local pending = tern.kv.get(KV_PENDING)
    if pending ~= nil then
        local own = ownVersion()
        local outcome = if own then releases.pendingOutcome(pending, own, os.time()) else "wait"
        if outcome == "updated" and hasProcessExit and not (pending :: any).done
            and os.time() - (pending :: any).at <= releases.PENDING_TTL then
            -- The step may still fail after it copied the plugin: its end tells, unless the old VM saw it end well. A step that
            -- never reported (Tern closed while it ran) leaves the key to the load after the 10 minutes, which reports the update.
            outcome = "wait"
        end
        if outcome == "updated" then
            tern.kv.set(KV_PENDING, nil)
            cx:toast("info", "tern-kata plugin updated to v" .. (pending :: any).version)
        elseif outcome == "drop" then
            tern.kv.set(KV_PENDING, nil)
        end
    end
    releaseCheck()
end

-- A plugin step still running when this plugin reloaded (its install replaced the files) ends as an event here, since its
-- callback went with the old VM. Tern without the event leaves the outcome to onLoad. While the key is set an event is
-- the step's end only when its stdout holds the step's marker line: other children (`tern plugin dir`) end the same way.
-- Clearing the key here also keeps onLoad, which runs on the same load, from reporting a success a second time.
hasProcessExit = pcall(function()
    tern.on("process_exit", function(ev, cx)
        local pending: any = tern.kv.get(KV_PENDING)
        if pending == nil or not ev.stdout:find(releases.STEP_MARKER, 1, true) then return end
        tern.kv.set(KV_PENDING, nil)
        if ev.status == 0 then
            cx:toast("info", "tern-kata plugin updated to v" .. tostring(pending.version))
        else
            local line = lastStderrLine(ev.stderr)
            cx:toast("error", "tern-kata plugin was not updated", line ~= "" and line or ("the install script exited with status " .. tostring(ev.status)))
        end
    end)
end)

tern.on("window_start", onLoad)
tern.timer(1, function(cx)
    if cx then onLoad(cx) end
end)

tern.command({
    id = "update-checks",
    title = "Turn tern-kata update checks off/on",
    run = function(cx)
        withPluginDir(function()
            local off, text = releases.toggle(currentCheck())
            tern.kv.set(KV_OFF, off)
            cx:toast("info", text)
        end)
    end,
})

tern.command({
    id = OPEN_ID,
    title = "Open kata",
    keys = {"f9"},
    run = function(cx) open(cx, nil) end,
})

tern.command({
    id = "wrap-up-agents",
    title = "Wrap up every running agent",
    run = function(cx)
        openKata(cx, true, function(pane) paneAction(cx, pane, "wrap-up-sweep", "") end, function(pane) owe(pane, "wrap-up-sweep", "") end)
    end,
})

-- The install one-liners per shell family; the session host's OS is not visible from the window, so the user picks.
-- The trailing read keeps the Pane open to show the script's output (a clean exit closes the Pane).
local installScripts = panecommands.SCRIPTS
local installChoices: { PickItem } = {
    {label = "Linux or macOS (sh)", command = "curl -fsSL " .. installScripts .. "install.sh | sh",
        pause = "printf 'Press Enter to close '; read _"},
    {label = "Windows (PowerShell)", command = panecommands.INSTALL_WINDOWS},
}
for _, choice in installChoices do choice.detail = choice.command end

tern.command({
    id = "install",
    title = "Install tern-kata",
    run = function(cx)
        cx:pick({
            items = installChoices,
            placeholder = "Run the install script in a new Pane on the session host",
            action = "install",
        }):next(function(choice, err, cx2)
            if not choice or not cx2 then return end
            launch(cx2, choice.pause and (choice.command .. "; " .. choice.pause) or choice.command)
        end)
    end,
})

-- Update tern-kata: the session host first, in a Pane on the focused Pane's host. The window machine's plugin follows when
-- the update ended well and that host is another machine; on the window's own machine the update brought the plugin along.
local function updatePlugin(cx: WindowCx, version: string)
    tern.kv.set(KV_PENDING, {version = version, at = os.time()})
    local argv, env, stdin = releases.pluginStep(version, tern.runtime.os, tern.getenv("TERN_KATA_BASE_URL"))
    -- Tern starts the process in this plugin's folder, which the install replaces: on Windows a folder a process works in
    -- can't be replaced ("os error 32"), so the step runs in the home folder.
    local home = tern.getenv(if tern.runtime.os == "windows" then "USERPROFILE" else "HOME")
    local ok = pcall(function()
        tern.process.run(argv, {cwd = home, env = env, stdin = stdin, timeout_ms = 5 * 60 * 1000}, function(result, cx2)
            -- Success reloads the plugin, so this callback usually never runs; the new one reports it. A step that ended
            -- before the reload reached this VM marks the key done, which tells the new load the step ended well.
            if result.status == 0 then
                local pending: any = tern.kv.get(KV_PENDING)
                if type(pending) == "table" then
                    pending.done = true
                    tern.kv.set(KV_PENDING, pending)
                end
                return
            end
            tern.kv.set(KV_PENDING, nil)
            if cx2 then
                local line = lastStderrLine(result.stderr)
                cx2:toast("error", "tern-kata plugin was not updated", line ~= "" and line or ("the install script exited with status " .. result.status))
            end
        end)
    end)
    if not ok then
        tern.kv.set(KV_PENDING, nil)
        cx:toast("error", "tern-kata was updated on the session host, but this device's plugin wasn't", "Update the plugin on a machine that can run the install script")
    end
end

-- Polls once a second until the update's Pane ends: a non-zero exit marks it exited and the Pane stays to be read (a
-- failure); a Pane that is gone, or marked exited with status 0, is success. A window plugin already at the release's
-- version is not installed again, as `tern-kata update` leaves a plugin copy of the same version alone, and a linked one
-- (a checkout) is left as it is, as the install script would.
local function watchUpdate(pane: number, version: string, remote: boolean)
    tern.timer(1000, function(cx)
        if not cx then return end
        local info = cx.session:pane(pane)
        if info and info.exited ~= nil and info.exited ~= 0 then return end
        if info and info.exited == nil then return watchUpdate(pane, version, remote) end
        if not remote then return end
        if ownVersion() == version then
            cx:toast("info", "tern-kata plugin v" .. version .. " is up to date")
            return
        end
        withPluginDir(function()
            if releases.isLinked(tern.plugin.dir, pluginDir) then
                cx:toast("info", "tern-kata plugin is linked, left as it is")
                return
            end
            updatePlugin(cx, version)
        end)
    end)
end

tern.command({
    id = "update",
    title = "Update tern-kata",
    run = function(cx)
        local remote = cx.hosts:current().kind == "remote"
        tern.fetch(versionTextUrl(), {timeout_ms = 15000}, function(result, cx2)
            if not cx2 then return end
            local version = if result.error == nil and result.status == 200 then releases.release(result.body) else nil
            if not version then
                cx2:toast("error", "tern-kata was not updated", "Could not read version.txt of the latest release")
                return
            end
            local pane = launch(cx2, panecommands.UPDATE)
            if pane then
                changeUpdatePanes(pane, true)
                watchUpdate(pane, version, remote)
            end
        end)
    end,
})

tern.route.link(function(link, cx)
    local project, shortID = link.url:match("^kata://([^/]+)/([^/?#]+)$")
    -- Tern's chat markdown links https but not custom schemes; .invalid never resolves, so only this route opens it.
    if not project then project, shortID = link.url:match("^https://kata%.invalid/([^/]+)/([^/?#]+)$") end
    -- References are literal URL segments: encoded or ambiguous separators are declined.
    if not project or not shortID or project:find("[?#%%\\]") or shortID:find("[%%\\]")
        or (project .. shortID):find("[%z\r\n]") then return nil end
    open(cx, project .. "/" .. shortID)
    return {handled = true}
end)
