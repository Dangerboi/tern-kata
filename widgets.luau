--!strict

-- What the widgets of the widget bar have in common, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/widgets_test.luau`. `capped` is the "at most five, then how many more" list of a tooltip; `merge` is what the
-- palette command writes back to the bar's `widgets` setting.

-- What a widget's `render` returns, the part of tern.d.luau's WidgetView these widgets use.
export type WidgetView = { text: string, tone: string?, title: string?, command: string?, value: number? }

local M = {}

local CAPPED_LINES = 5

-- `lines` as they are when there are at most five; otherwise the first five and "and N more".
function M.capped(lines: { string }): { string }
    local shown: { string } = {}
    for index = 1, math.min(#lines, CAPPED_LINES) do table.insert(shown, lines[index]) end
    if #lines > CAPPED_LINES then table.insert(shown, "and " .. (#lines - CAPPED_LINES) .. " more") end
    return shown
end

-- The bar's `widgets` list with `tern-kata/<id>` appended for each of `ids` it lacks; every other entry stays where it is,
-- as it is. An entry is a table with an `id` or a plain string, either of which may carry an `@style` suffix. Returns the
-- new list and the ids that were added; the list given is not changed.
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
        local full = "tern-kata/" .. id
        if not present[full] then
            present[full] = true
            table.insert(merged, {id = full})
            table.insert(added, id)
        end
    end
    return merged, added
end

return M
