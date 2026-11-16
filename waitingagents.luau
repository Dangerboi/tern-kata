--!strict

-- Which kata agent is waiting for you, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/waitingagents_test.luau`. window.luau keeps the order in which agents began waiting and asks `waiting` for the
-- ones that count; the Go to the agent waiting for you command goes to the first.

export type Agent = { pane: number, command: string?, state: string }

local M = {}

-- The agents kata started that wait for the user, the one that began waiting last first. `order` gives each pane the number
-- of its move into waiting (a larger number is later); a pane without one (it waited before this plugin loaded) follows the
-- numbered panes, in `agents` order. `isKata` says whether a command names a tern-kata prompt file.
function M.waiting(agents: { Agent }, order: { [number]: number }, isKata: (string?) -> boolean): { number }
    local timed: { number } = {}
    local untimed: { number } = {}
    for _, agent in agents do
        if agent.state == "waiting_input" and isKata(agent.command) then
            table.insert(if order[agent.pane] ~= nil then timed else untimed, agent.pane)
        end
    end
    table.sort(timed, function(a, b) return order[a] > order[b] end)
    for _, pane in untimed do table.insert(timed, pane) end
    return timed
end

return M
