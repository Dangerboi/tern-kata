--!strict

-- What the widgets of the widget bar have in common, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/widgets_test.luau`.

local M = {}

local CAPPED_LINES = 5
-- Tern cuts a tooltip line at about this many characters, whatever follows.
local LINE_CHARS = 45

-- The id Tern gives a command a plugin registers under `name`.
function M.commandId(plugin: string, name: string): string
    return "plugin." .. plugin .. "." .. name
end

-- The line itself when it fits a tooltip line, else its first characters and an ellipsis.
function M.fit(line: string): string
    local length = utf8.len(line)
    if length == nil then return if #line <= LINE_CHARS then line else line:sub(1, LINE_CHARS - 1) .. "…" end
    if length <= LINE_CHARS then return line end
    return line:sub(1, utf8.offset(line, LINE_CHARS) - 1) .. "…"
end

-- At most CAPPED_LINES lines, each fitted, then how many more there were.
function M.capped(lines: { string }): { string }
    local shown: { string } = {}
    for index = 1, math.min(#lines, CAPPED_LINES) do table.insert(shown, M.fit(lines[index])) end
    if #lines > CAPPED_LINES then table.insert(shown, "and " .. (#lines - CAPPED_LINES) .. " more") end
    return shown
end

-- The bar's `widgets` list with each of `ids` (full widget ids, `<plugin>/<id>`) appended when it lacks it; every other entry
-- stays where it is, as it is. An entry is a table with an `id` or a plain string, either of which may carry an `@style`
-- suffix.
function M.merge(list: { any }, ids: { string }): ({ any }, { string })
    local merged: { any } = {}
    local present: { [string]: boolean } = {}
    for _, entry in list do
        table.insert(merged, entry)
        local id = if type(entry) == "table" then entry.id elseif type(entry) == "string" then entry else nil
        if type(id) == "string" then present[(id:match("^[^@]*") :: string)] = true end
    end
    local added: { string } = {}
    for _, id in ids do
        if not present[id] then
            present[id] = true
            table.insert(merged, {id = id})
            table.insert(added, id)
        end
    end
    return merged, added
end

return M
