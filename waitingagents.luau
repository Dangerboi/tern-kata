--!strict

-- Which kata agent is waiting for you, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/waitingagents_test.luau`. window.luau keeps the order in which agents began waiting and asks `waiting` for the
-- ones that count (the first is the one to go to) and `segment` for the status line.

export type StatusSegment = { text: string, icon: string?, tone: string?, command: string? }
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

-- The status-line segments for `count` waiting agents: none for 0.
function M.segment(count: number, command: string): { StatusSegment }?
    if count <= 0 then return nil end
    return {{text = count .. " waiting", tone = "warning", command = command}}
end

return M
