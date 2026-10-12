--!strict

local kataPane: number? = nil
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

local function existingPane(cx: WindowCx): number?
    if kataPane and isKata(cx.session:pane(kataPane)) then return kataPane end
    for _, info in cx.session:panes() do
        if isKata(info) then
            kataPane = info.pane
            return kataPane
        end
    end
    kataPane = nil
    return nil
end

local function selectIssue(cx: WindowCx, pane: number, ref: string)
    cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = "select-issue", value = ref})
end

local function deliver(cx: WindowCx, pane: number)
    local ref = pending[pane]
    if ref and cx.session:surface(pane) then
        pending[pane] = nil
        selectIssue(cx, pane, ref)
    end
end

-- Startup-failure rule, the plugin's half of the contract with cmd/tern-kata: a kata Pane failing within 2 s with
-- status 2 (Go panic) or 3 (the binary's own error) means tern-kata ran and failed. Status 1 can be either: it is
-- PowerShell's not-found code, and on Windows Tern reports every failing launch as 1, so tern-kata's 3 never arrives.
-- Any other failing status is the shell's own not-found code (127, 9009), so it is not installed or not on PATH.
local function exitedRightAway(cx: WindowCx, pane: number, status: number)
    fresh[pane] = nil
    if status == 0 then return end
    if status == 2 or status == 3 then
        cx:toast("error", "tern-kata failed to start", "Run tern-kata in a shell to see why")
    elseif status == 1 then
        cx:toast("error", "tern-kata didn't start", "If it isn't installed: go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest. Otherwise run tern-kata in a shell to see why")
    else
        cx:toast("error", "tern-kata is not installed or not on PATH", "go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest")
    end
end

-- After the window, a failing exit still closes the Pane (spec), silently: poll once a second until it is gone or replaced.
local function reap(pane: number)
    tern.timer(1000, function(cx)
        if not cx or kataPane ~= pane then return end
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

local function open(cx: WindowCx, ref: string?)
    if ref then
        local pane = existingPane(cx)
        if pane then
            cx.layout:focus(pane)
            selectIssue(cx, pane, ref)
            return
        end
    end
    local target = cx.session:focused()
    local pane
    if target then
        pane = cx.layout:split(target, "right", {command = "tern-kata"})
    else
        pane = cx.layout:new_tab({command = "tern-kata"})
    end
    if not pane then return end
    kataPane = pane
    pending[pane] = ref
    watch(pane, 20)
end

tern.on("pane_closed", function(ev, cx)
    if kataPane == ev.pane then kataPane = nil end
    pending[ev.pane] = nil
end)

tern.command({
    id = "open",
    title = "Open kata",
    keys = {"f9"},
    run = function(cx) open(cx, nil) end,
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
