--!strict

-- kata Panes this plugin launched, until they close: each is reaped on its own, since every tab may have one.
local launched: { [number]: boolean } = {}
local fresh: { [number]: boolean } = {}
-- Issue reference per new kata Pane, sent over TSP once its surface is live, so it never passes through a shell.
local pending: { [number]: string? } = {}

-- A kata Pane has the title `tern-kata` (which the binary sets), or a program whose file name is tern-kata or
-- tern-kata.exe, split on either path separator so Windows paths match. A longer name merely containing it does not.
local function isKataProgram(program: string?): boolean
    if program == nil then return false end
    local name = (program:match("[^/\\]*$") or ""):lower()
    return name == "tern-kata" or name == "tern-kata.exe"
end

local function isKata(info: PaneInfo?): boolean
    return info ~= nil and info.exited == nil and (info.title == "tern-kata" or isKataProgram(info.program))
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

local function restyleMarks()
    local faces: { string } = {}
    for pane in marked do table.insert(faces, string.format(".tn-face[data-pane='%d']", pane)) end
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

local function selectIssue(cx: WindowCx, pane: number, ref: string)
    cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = "select-issue", value = ref})
end

-- kata Panes that have been sent the agent states since their surface went live.
local relayed: { [number]: boolean } = {}

-- Background agents live in a session of this name (internal/model AgentSession), kept out of sight.
local AGENT_SESSION = "kata agents"

-- The tabs that are in that session, by id.
local function hiddenTabs(cx: WindowCx): { [number]: boolean }
    local sessions: { [number]: boolean } = {}
    for _, session in cx.sessions:list() do
        if session.name == AGENT_SESSION then sessions[session.id] = true end
    end
    local tabs: { [number]: boolean } = {}
    for _, tab in cx.session:tabs() do
        if sessions[tab.session] then tabs[tab.id] = true end
    end
    return tabs
end

-- Every agent block in the session as JSON for the Pane, which matches it against its own agents. The pane id goes as
-- decimal text; %.0f prints a whole number without an exponent. Tern's API gives the id only as a Luau number (a double),
-- so the text is exact only for ids up to 2^53: a larger id is already rounded here and matches no agent of the Pane.
-- A window on another machine numbers panes differently from the session host, so the id may match nothing there; the command
-- (it names the agent's prompt file) lets the Pane find its agent anyway. A missing last message or command is the empty string.
-- `hidden` says whether the agent's tab is in the hidden session, so the Pane learns when an agent is opened or put away
-- without it; it is left out when the window cannot place the agent's tab.
local function agentStates(cx: WindowCx): string
    local list = tern.json.array({})
    local hidden = hiddenTabs(cx)
    for _, agent in cx.agents:list() do
        local last = agent.last_message
        local tab = cx.session:tab_of(agent.pane)
        table.insert(list, {
            pane = string.format("%.0f", agent.pane),
            state = agent.state,
            last_message = if type(last) == "string" then last else "",
            command = if type(agent.command) == "string" then agent.command else "",
            hidden = if tab then hidden[tab] == true else nil,
        })
    end
    return tern.json.encode(list)
end

local function relayStates(cx: WindowCx, pane: number, states: string)
    if not cx.session:surface(pane) then return end
    relayed[pane] = true
    cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = "agent-states", value = states})
end

-- Places new agent tabs; defined with the tab placement below, and run on every relay pass.
local placeAgents: (WindowCx) -> ()

-- The states to every live kata Pane, or only to those not sent them yet.
local function relayAll(cx: WindowCx, onlyNew: boolean)
    placeAgents(cx)
    local states: string? = nil
    for _, info in cx.session:panes() do
        if isKata(info) and not (onlyNew and relayed[info.pane]) and cx.session:surface(info.pane) then
            if not states then states = agentStates(cx) end
            relayStates(cx, info.pane, states)
        end
    end
end

local function deliver(cx: WindowCx, pane: number)
    local ref = pending[pane]
    if ref and cx.session:surface(pane) then
        pending[pane] = nil
        selectIssue(cx, pane, ref)
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
        cx:toast("error", "tern-kata failed to start", "Run tern-kata in a shell to see why")
    elseif status == 1 then
        cx:toast("error", "tern-kata didn't start", "If it isn't installed, run Install tern-kata from the command palette (or the install script in the README). Otherwise run tern-kata in a shell to see why")
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

local function open(cx: WindowCx, ref: string?)
    if ref then
        local pane = existingPane(cx)
        if pane then
            cx.layout:focus(pane)
            selectIssue(cx, pane, ref)
            return
        end
    end
    local pane = launch(cx, "tern-kata")
    if not pane then return end
    launched[pane] = true
    pending[pane] = ref
    watch(pane, 20)
end

-- `W` starts an agent in a new tab, which Tern appends as the last tab. The agent's prompt file is named
-- `prompt-<starter>-<random>.md` (internal/data), `<starter>` being the block of the starting kata Pane, and that name is in the
-- command line Tern reports for the new tab's Pane: the agent block's `command`, else the running line. The tab is moved right of the
-- starting Pane's tab; no other tab is reordered.
-- Two ways find a new agent tab. `tab_created` fires for tabs opened on this window's own machine. A window on another machine
-- gets no `tab_created` for a tab the CLI opens on the session host, and numbers panes differently from the host, so
-- `<starter>` is no pane id there. Such a window places on `agent_state` and on every relay pass instead: an agent pane whose
-- command carries the marker is placed once (remembered in `placed` until its pane closes) and then never touched again.
-- The starter is the kata Pane whose id is `<starter>`, else the focused Pane if it is a kata Pane (the Pane the user just pressed
-- `W` in), else the window's only kata Pane; with none of these nothing moves.
local PLACE_TRIES = 10
local placed: { [number]: boolean } = {}

local function markerOf(text: string?): number?
    local block = text and text:match("prompt%-(%d+)%-%w+%.md")
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

-- The kata Pane an agent started from, as far as this window can tell: see the comment above.
local function starterPane(cx: WindowCx, starter: number): PaneInfo?
    local own = cx.session:pane(starter)
    if isKata(own) then return own end
    local focused = cx.session:focused()
    local info = focused and cx.session:pane(focused)
    if isKata(info) then return info end
    local only: PaneInfo? = nil
    for _, candidate in cx.session:panes() do
        if isKata(candidate) then
            if only then return nil end
            only = candidate
        end
    end
    return only
end

-- Moves `tab` right of the tab of `home`, the starting Pane; false when there is none.
local function placeTab(cx: WindowCx, tab: number, home: PaneInfo?): boolean
    if not home then return false end
    if not home.tab or home.tab == tab then return true end
    local starterTab: TabInfo? = nil
    local newTab: TabInfo? = nil
    for _, info in cx.session:tabs() do
        if info.id == home.tab then starterTab = info end
        if info.id == tab then newTab = info end
    end
    if not starterTab or not newTab or starterTab.session ~= newTab.session then return true end
    if newTab.position ~= starterTab.position + 1 then
        cx.layout:order_tab(tab, starterTab.position + 1)
    end
    return true
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
    if hiddenTabs(cx)[tab] then
        for _, info in cx.session:panes() do
            if info.tab == tab then placed[info.pane] = true end
        end
        return
    end
    -- Here the starter's own id is exact, since the tab is on this window's machine; a starter that is gone leaves the tab last.
    placeTab(cx, tab, cx.session:pane(starter))
    for _, info in cx.session:panes() do
        if info.tab == tab then placed[info.pane] = true end
    end
end

-- Agent panes not placed yet whose command carries the marker, each placed once. A background agent's tab is in the hidden
-- session, which is not ordered by the Pane that started it, so such an agent is never placed.
placeAgents = function(cx: WindowCx)
    local hidden: { [number]: boolean }? = nil
    for _, agent in cx.agents:list() do
        if not placed[agent.pane] then
            local starter = markerOf(agent.command)
            local info = if starter then cx.session:pane(agent.pane) else nil
            if starter and info and info.tab then
                local tabs = hidden or hiddenTabs(cx)
                hidden = tabs
                if tabs[info.tab] then
                    placed[agent.pane] = true
                elseif placeTab(cx, info.tab, starterPane(cx, starter)) then
                    placed[agent.pane] = true
                end
            end
        end
    end
end

-- Tern's notification for an agent that waits for the user focuses the agent where it lives, and a background agent lives in
-- the hidden session, which is meant to stay out of sight: the user would land in a session of nothing but agents. So focus
-- in that session opens the agent beside a kata Pane, as a click on its Agent badge does, and shows that Pane's session. The
-- kata Pane is the one the agent's prompt file names (this window's own Pane, when the id is exact), else the one this window
-- focused last, else its only one; with none the agent stays. The move is itself a focus, but then the agent is out of the
-- hidden session and nothing happens. The relay that follows tells the Pane at once, so it records the agent as open.
local lastKata: number? = nil

local function kataPaneFor(cx: WindowCx, starter: number?): PaneInfo?
    local own = if starter then cx.session:pane(starter) else nil
    if isKata(own) then return own end
    local last = if lastKata then cx.session:pane(lastKata) else nil
    if isKata(last) then return last end
    local only: PaneInfo? = nil
    for _, candidate in cx.session:panes() do
        if isKata(candidate) then
            if only then return nil end
            only = candidate
        end
    end
    return only
end

local function isPromptCommand(command: string?): boolean
    return command ~= nil and command:find("agent%-prompts[/\\]prompt%-") ~= nil
end

local openingAgent = false

local function openFocusedAgent(cx: WindowCx, pane: number, tab: number)
    if openingAgent or not hiddenTabs(cx)[tab] then return end
    local command: string? = nil
    for _, agent in cx.agents:list() do
        if agent.pane == pane then command = agent.command end
    end
    if not isPromptCommand(command) then return end
    local kata = kataPaneFor(cx, markerOf(command))
    if not kata or not kata.tab then return end
    openingAgent = true
    local moved = cx.layout:move(pane, kata.pane, "right")
    openingAgent = false
    if not moved then return end
    for _, info in cx.session:tabs() do
        if info.id == kata.tab then cx.sessions:switch(info.session) end
    end
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

tern.on("tab_created", function(ev, cx)
    placeAgentTab(cx, ev.tab, PLACE_TRIES)
end)

tern.on("pane_closed", function(ev, cx)
    launched[ev.pane] = nil
    pending[ev.pane] = nil
    relayed[ev.pane] = nil
    placed[ev.pane] = nil
    if lastKata == ev.pane then lastKata = nil end
    if marked[ev.pane] then
        marked[ev.pane] = nil
        restyleMarks()
    end
end)

local function markAll(cx: WindowCx)
    for _, info in cx.session:panes() do
        if isKata(info) then marked[info.pane] = true end
    end
    restyleMarks()
    -- Agents already there were placed (or put somewhere on purpose) before this run: only later ones are moved.
    for _, agent in cx.agents:list() do placed[agent.pane] = true end
    relayAll(cx, false)
end

-- The kata Panes already open: at window start, and after a plugin reload, which runs this file again but not window_start.
tern.on("window_start", markAll)
tern.timer(1, function(cx)
    if cx then markAll(cx) end
end)

-- tern-kata sets its title as it starts, so a title event finds a kata Pane started from any shell; other titles, such as an
-- agent's spinner, cost no lookup.
tern.on("title", function(ev, cx)
    if ev.title == "tern-kata" or marked[ev.pane] then
        markIf(cx, ev.pane)
        if marked[ev.pane] and not relayed[ev.pane] then relayAll(cx, true) end
    end
end)

-- Agents' states go to every kata Pane as they change, so its badges follow them.
tern.on("agent_state", function(ev, cx)
    relayAll(cx, false)
end)

tern.command({
    id = "open",
    title = "Open kata",
    keys = {"f9"},
    run = function(cx) open(cx, nil) end,
})

-- The install one-liners per shell family; the session host's OS is not visible from the window, so the user picks.
-- The trailing read keeps the Pane open to show the script's output (a clean exit closes the Pane).
local installScripts = "https://raw.githubusercontent.com/Dangerboi/tern-kata/main/"
local installChoices: { PickItem } = {
    {label = "Linux or macOS (sh)", command = "curl -fsSL " .. installScripts .. "install.sh | sh",
        pause = "printf 'Press Enter to close '; read _"},
    -- Wrapped in powershell so it also works when the Pane's default shell is cmd, where `;` is no separator.
    {label = "Windows (PowerShell)",
        command = "powershell -NoProfile -ExecutionPolicy Bypass -Command \"irm " .. installScripts
            .. "install.ps1 | iex; Read-Host 'Press Enter to close'\""},
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
