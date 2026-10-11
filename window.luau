local kataPane = nil
local fresh = {}
-- Issue reference per new kata Pane, sent over TSP once its surface is live, so it never passes through a shell.
local pending = {}

local function isKata(info)
    return info and info.exited == nil
        and (info.title == "tern-kata" or info.program == "tern-kata"
            or (info.program and info.program:match("/tern%-kata$")))
end

local function existingPane(cx)
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

local function selectIssue(cx, pane, ref)
    cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = "select-issue", value = ref})
end

local function deliver(cx, pane)
    local ref = pending[pane]
    if ref and cx.session:surface(pane) then
        pending[pane] = nil
        selectIssue(cx, pane, ref)
    end
end

local function exitedRightAway(cx, pane)
    fresh[pane] = nil
    cx:toast("error", "tern-kata exited right away",
        "Is it installed on the session host? go install github.com/Dangerboi/tern-kata@latest")
end

-- After the window, a failing exit still closes the Pane (spec), silently: poll once a second until it is gone or replaced.
local function reap(pane)
    tern.timer(1000, function(cx)
        if not cx or kataPane ~= pane then return end
        local info = cx.session:pane(pane)
        if not info then return end
        if info.exited ~= nil then return cx.layout:close(pane) end
        deliver(cx, pane)
        reap(pane)
    end)
end

-- A kata Pane gone or exited within the first 2 s (20 ticks of 100 ms) means the binary is missing or failed at
-- startup. Tern closes the Pane on a clean exit but keeps it, marked exited, on a failing one, so close it here.
local function watch(pane, ticks)
    fresh[pane] = true
    tern.timer(100, function(cx)
        if not cx or not fresh[pane] then return end
        local info = cx.session:pane(pane)
        if not info then return exitedRightAway(cx, pane) end
        if info.exited ~= nil then
            exitedRightAway(cx, pane)
            return cx.layout:close(pane)
        end
        deliver(cx, pane)
        if ticks > 1 then watch(pane, ticks - 1) else fresh[pane] = nil; reap(pane) end
    end)
end

local function open(cx, ref)
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
    if fresh[ev.pane] then exitedRightAway(cx, ev.pane) end
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
    if not project or project:find("[?#%%\\]") or shortID:find("[%%\\]")
        or (project .. shortID):find("[%z\r\n]") then return nil end
    open(cx, project .. "/" .. shortID)
    return {handled = true}
end)
