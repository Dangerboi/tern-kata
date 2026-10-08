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
    local target = cx.session:focused()
    if target then
        kataPane = cx.layout:split(target, "right")
    else
        kataPane = cx.layout:new_tab()
    end
    if not kataPane then return end
    local launch = "exec tern-kata"
    if ref then launch ..= " --issue " .. quote(ref) end
    local command = "if command -v tern-kata >/dev/null 2>&1; then " .. launch
        .. "; else echo " .. quote("go install github.com/Dangerboi/tern-kata@latest") .. "; fi"
    cx:run(kataPane, "\r" .. command .. "\r")
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
