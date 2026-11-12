--!strict

-- Which kata agent is waiting for you, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/waitingagents_test.luau`. window.luau keeps the order in which agents began waiting and asks `waiting` for the
-- ones that count (the first is the one to go to), `segment` for the "N waiting" text in the widget bar's Plugin segments
-- widget, and `widget` for the Waiting kata agents widget a user places in the bar. `mergeWidgets` is what the palette
-- command writes back to the bar's `widgets` setting.

export type StatusSegment = { text: string, icon: string?, tone: string?, command: string? }
export type WidgetView = { text: string, tone: string?, title: string?, command: string? }
export type Agent = { pane: number, command: string?, state: string }

local M = {}

-- The agents kata started that wait for the user, the one that began waiting last first. `order` gives each pane the number
-- of its move into waiting (a larger number is later); a pane without one (it waited before this plugin loaded) follows the
-- numbered panes, in `agents` order. `isKata` says whether a command names a tern-kata prompt file; `skip` is a pane that is closing.
function M.waiting(agents: { Agent }, order: { [number]: number }, isKata: (string?) -> boolean, skip: number?): { number }
    local timed: { number } = {}
    local untimed: { number } = {}
    for _, agent in agents do
        if agent.state == "waiting_input" and agent.pane ~= skip and isKata(agent.command) then
            table.insert(if order[agent.pane] ~= nil then timed else untimed, agent.pane)
        end
    end
    table.sort(timed, function(a, b) return order[a] > order[b] end)
    for _, pane in untimed do table.insert(timed, pane) end
    return timed
end

function M.segment(count: number, command: string): { StatusSegment }?
    if count <= 0 then return nil end
    return {{text = count .. " waiting", tone = "warning", command = command}}
end

local TOOLTIP_NAMES = 5

-- The tooltip of the widget: the waiting agents, newest first, at most five, then how many more. `titles` is every waiting
-- agent's block title in that order.
function M.tooltip(titles: { string }): string
    local lines: { string } = {}
    for index = 1, math.min(#titles, TOOLTIP_NAMES) do table.insert(lines, titles[index]) end
    if #titles > TOOLTIP_NAMES then table.insert(lines, "and " .. (#titles - TOOLTIP_NAMES) .. " more") end
    return table.concat(lines, "\n")
end

-- What the widget shows: nothing while no agent waits.
function M.widget(count: number, tooltip: string, command: string): WidgetView?
    if count <= 0 then return nil end
    return {text = count .. " waiting", tone = "warning", title = tooltip, command = command}
end

-- The bar's `widgets` list with `<plugin>/<id>` appended for each of `ids` it lacks; every other entry stays where it is,
-- as it is. An entry is a table with an `id` or a plain string, either of which may carry an `@style` suffix. Returns the
-- new list and the ids that were added; the list given is not changed.
function M.mergeWidgets(list: { any }, ids: { string }, plugin: string): ({ any }, { string })
    local merged: { any } = {}
    local present: { [string]: boolean } = {}
    for _, entry in list do
        table.insert(merged, entry)
        local id = if type(entry) == "table" then entry.id elseif type(entry) == "string" then entry else nil
        if type(id) == "string" then present[(id:match("^[^@]*") :: string)] = true end
    end
    local added: { string } = {}
    for _, id in ids do
        local full = plugin .. "/" .. id
        if not present[full] then
            present[full] = true
            table.insert(merged, {id = full})
            table.insert(added, id)
        end
    end
    return merged, added
end

return M
