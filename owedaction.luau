--!strict

-- Whether a click on a kata link or menu is delivered to a kata Pane already in the tab or opens a new one, as plain data: no
-- Tern call is made here, so the rule is tested by `luau test/owedaction_test.luau`. window.luau asks `route` and carries the
-- answer out.

export type Route = "queue" | "launch"

local M = {}

-- `waiting`: the plugin itself is waiting on this Pane to go live (it launched it, or is retrying its surface). `running`: kata
-- runs in it (window.luau `live`). The action is queued on the Pane and sent as soon as kata runs there: at once for a running
-- Pane, when the Pane starts for one the plugin waits on. Any other Pane found by its title alone may be a shell where kata has
-- quit, and a queued action would run whenever kata next started there, so such a Pane is not reused: a new Pane is launched.
function M.route(waiting: boolean, running: boolean): Route
    if waiting or running then return "queue" end
    return "launch"
end

return M
