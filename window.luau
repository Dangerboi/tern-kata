local kataPane = nil

local function quote(value)
    return "'" .. value:gsub("'", "'\\''") .. "'"
end

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

local function open(cx, ref)
    if ref then
        local pane = existingPane(cx)
        if pane then
            cx.layout:focus(pane)
            cx.session:event(pane, {ev = "action", id = "main.columns.list.issues", act = "select-issue", value = ref})
            return
        end
    end
    tern.process.run({"sh", "-c", "command -v tern-kata"}, {timeout_ms = 2000}, function(result, live)
        if not live then return end
        local executable = result.stdout:match("^([^\r\n]+)")
        if result.status ~= 0 or not executable then
            live:toast("error", "tern-kata is not on PATH", "go install github.com/Dangerboi/tern-kata@latest")
            return
        end
        -- Launch.command captures the login environment synchronously inside the hook.
        local command = "exec " .. quote(executable)
        if ref then command ..= " --issue " .. quote(ref) end
        local target = live.session:focused()
        if target then
            kataPane = live.layout:split(target, "right")
        else
            kataPane = live.layout:new_tab()
        end
        if kataPane then live:run(kataPane, command .. "\r") end
    end)
end

tern.command({
    id = "open",
    title = "Open kata",
    keys = {"f9"},
    run = function(cx) open(cx, nil) end,
})

tern.route.link(function(link, cx)
    local project, shortID = link.url:match("^kata://([^/]+)/([^/?#]+)$")
    -- References are literal URL segments: encoded or ambiguous separators are declined.
    if not project or project:find("[?#%%\\]") or shortID:find("[%%\\]")
        or (project .. shortID):find("[%z\r\n]") then return nil end
    open(cx, project .. "/" .. shortID)
    return {handled = true}
end)
