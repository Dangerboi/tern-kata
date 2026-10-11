local kataPane = nil
local fresh = {}

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
        if ticks > 1 then watch(pane, ticks - 1) else fresh[pane] = nil; reap(pane) end
    end)
end

local function open(cx, ref)
    if ref then
        local pane = existingPane(cx)
        if pane then
            cx.layout:focus(pane)
            cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = "select-issue", value = ref})
            return
        end
    end
    -- The command is unquoted launch text: callers pass only [A-Za-z0-9._-] references.
    local command = ref and "tern-kata --issue " .. ref or "tern-kata"
    local target = cx.session:focused()
    local pane
    if target then
        pane = cx.layout:split(target, "right", {command = command})
    else
        pane = cx.layout:new_tab({command = command})
    end
    if not pane then return end
    kataPane = pane
    watch(pane, 20)
end

tern.on("pane_closed", function(ev, cx)
    if kataPane == ev.pane then kataPane = nil end
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
    -- The reference goes into an unquoted launch command, so only shell-inert characters pass.
    if not project or project:find("[^A-Za-z0-9._-]") or shortID:find("[^A-Za-z0-9._-]") then return nil end
    open(cx, project .. "/" .. shortID)
    return {handled = true}
end)
