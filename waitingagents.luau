--!strict

-- Which kata agent is waiting for you, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/waitingagents_test.luau`. window.luau keeps the order in which agents began waiting and asks `waiting` for the
-- ones that count (the first is the one to go to), `segment` for the "N waiting" text in the widget bar's Plugin segments
-- widget, and `widget` for the Waiting kata agents widget a user places in the bar.

export type StatusSegment = { text: string, icon: string?, tone: string?, command: string? }
export type WidgetView = { text: string, tone: string?, title: string?, command: string? }
export type Agent = { pane: number, command: string?, state: string }

local widgets = require("./widgets")

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

-- The tooltip of the widget: the waiting agents, newest first, at most five, then how many more. `titles` is every waiting
-- agent's block title in that order.
function M.tooltip(titles: { string }): string
    return table.concat(widgets.capped(titles), "\n")
end

-- What the widget shows: nothing while no agent waits.
function M.widget(count: number, tooltip: string, command: string): WidgetView?
    if count <= 0 then return nil end
    return {text = count .. " waiting", tone = "warning", title = tooltip, command = command}
end

return M
