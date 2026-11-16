--!strict

-- What a click on a kata link or menu does when a kata Pane is already in the tab, as plain data: no Tern call is made here, so
-- the rule is tested by `luau test/owedaction_test.luau`. window.luau asks `route` and carries the answer out.

export type Route = "owe" | "send" | "launch"

local M = {}

-- `waiting`: the plugin itself is waiting on this Pane to go live (it launched it, or is retrying its surface). `running`: kata has not quit in
-- it: its surface is not the retained one a Pane keeps after kata exits (a surface still to come counts as running).
-- An action is owed only to a Pane the plugin is waiting on: the Pane starts when it is ready. Any other Pane found by its
-- title alone may be a shell where kata has quit, and an owed action would run whenever kata next started there, so a running
-- Pane gets the action now and one where kata has quit is not reused: a new Pane is launched.
function M.route(waiting: boolean, running: boolean): Route
    if waiting then return "owe" end
    if running then return "send" end
    return "launch"
end

return M
