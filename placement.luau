--!strict

-- Where an agent's tab goes in a window's tab order, as plain data: no Tern call is made here, so the rule is tested
-- by `luau test/placement_test.luau`. window.luau takes one snapshot of the window per relay pass, asks `plan` for the moves
-- and makes them.

export type TabRec = { id: number, session: number, position: number }
-- `kata` says the pane is a live kata Pane. `marker` is the starting kata Pane's block named by the prompt file in the pane's
-- running command line, nil when the line names none.
export type PaneRec = { pane: number, tab: number?, kata: boolean, marker: number? }
-- `marker` is the same block, from the agent block's own command.
export type AgentRec = { pane: number, marker: number? }
export type Snapshot = {
    tabs: { TabRec },
    panes: { PaneRec },
    agents: { AgentRec },
    -- The tabs that are in the hidden agent session.
    hidden: { [number]: boolean },
    -- Panes handled before: never tried again.
    placed: { [number]: boolean },
    -- The pane the user is in, for an agent whose starter is no pane of this window.
    focused: number?,
}
export type Move = { tab: number, position: number }
export type Plan = {
    -- Panes tried in this pass, whatever the result; the caller remembers them as placed.
    tried: { number },
    -- In the order to make them.
    moves: { Move },
}

local M = {}

-- The kata Pane an agent started from, as far as the window can tell: the Pane whose id is `starter` (exact only on the
-- session host's machine), else `fallback` if it is a kata Pane, else the window's only kata Pane; nil with none of these.
function M.kataPaneFor(panes: { PaneRec }, starter: number?, fallback: number?): PaneRec?
    local own: PaneRec? = nil
    local other: PaneRec? = nil
    local only: PaneRec? = nil
    local many = false
    for _, info in panes do
        if info.kata then
            if info.pane == starter then own = info end
            if info.pane == fallback then other = info end
            if only then many = true end
            only = info
        end
    end
    if own then return own end
    if other then return other end
    if many then return nil end
    return only
end

-- The position that puts `tab` right of `home`, the tab of the starting Pane; nil when it is there already or in another session.
function M.positionBeside(home: TabRec, tab: TabRec): number?
    if home.id == tab.id or home.session ~= tab.session then return nil end
    if tab.position == home.position + 1 then return nil end
    return home.position + 1
end

-- The candidate tabs to move: those in the run of tabs at the end of their session made only of candidates and agent tabs not
-- handled yet (`pending`), when the tab directly before that run holds no kata Pane. Highest position first, so tabs moved to
-- the same home keep the order they were opened in.
local function unplacedTabs(
    tabs: { TabRec }, candidates: { [number]: boolean }, pending: { [number]: boolean }, kataTabs: { [number]: boolean }
): { number }
    local bySession: { [number]: { TabRec } } = {}
    for _, info in tabs do
        local list = bySession[info.session]
        if not list then
            list = {}
            bySession[info.session] = list
        end
        table.insert(list :: { TabRec }, info)
    end
    local found: { TabRec } = {}
    for _, list in bySession do
        table.sort(list, function(a, b) return a.position > b.position end)
        local run: { TabRec } = {}
        local before: TabRec? = nil
        for _, info in list do
            if candidates[info.id] or pending[info.id] then
                if candidates[info.id] then table.insert(run, info) end
            else
                before = info
                break
            end
        end
        if not (before and kataTabs[before.id]) then
            for _, info in run do table.insert(found, info) end
        end
    end
    table.sort(found, function(a, b) return a.position > b.position end)
    local ids: { number } = {}
    for _, info in found do table.insert(ids, info.id) end
    return ids
end

-- Reorders `tabs` as moving `tab` to `position` does: the tabs it passes shift by one.
local function applyMove(tabs: { TabRec }, tab: TabRec, position: number)
    local from = tab.position
    for _, other in tabs do
        if other.session == tab.session and other.id ~= tab.id then
            if position <= from and other.position >= position and other.position < from then
                other.position += 1
            elseif position > from and other.position > from and other.position <= position then
                other.position -= 1
            end
        end
    end
    tab.position = position
end

-- The agents to try in this pass and the tab moves they bring. An agent pane not in `placed` whose command carries the marker
-- is tried once, whatever the result; its tab is a candidate unless it is in the hidden session. `unplacedTabs` picks which
-- candidates move and `kataPaneFor` the marker gives the home. Rewrites each record's `position` in `snapshot.tabs` as the
-- moves are planned, so the caller passes records it will not use again.
function M.plan(snapshot: Snapshot): Plan
    local paneById: { [number]: PaneRec } = {}
    local pending: { [number]: boolean } = {}
    local kataTabs: { [number]: boolean } = {}
    for _, info in snapshot.panes do
        paneById[info.pane] = info
        if info.tab then
            if info.kata then kataTabs[info.tab] = true end
            if not snapshot.placed[info.pane] and info.marker ~= nil then pending[info.tab] = true end
        end
    end
    local tried: { number } = {}
    local candidates: { [number]: boolean } = {}
    local starters: { [number]: number } = {}
    for _, agent in snapshot.agents do
        local info = paneById[agent.pane]
        if not snapshot.placed[agent.pane] and agent.marker and info and info.tab then
            table.insert(tried, agent.pane)
            if not snapshot.hidden[info.tab] then
                candidates[info.tab] = true
                starters[info.tab] = agent.marker
            end
        end
    end
    local moves: { Move } = {}
    if next(candidates) == nil then return {tried = tried, moves = moves} end
    local tabs = snapshot.tabs
    local tabById: { [number]: TabRec } = {}
    for _, info in tabs do tabById[info.id] = info end
    for _, id in unplacedTabs(tabs, candidates, pending, kataTabs) do
        local home = M.kataPaneFor(snapshot.panes, starters[id], snapshot.focused)
        local homeTab = if home and home.tab then tabById[home.tab] else nil
        local tab = tabById[id]
        local position = if homeTab and tab then M.positionBeside(homeTab, tab) else nil
        if tab and position then
            table.insert(moves, {tab = id, position = position})
            applyMove(tabs, tab, position)
        end
    end
    return {tried = tried, moves = moves}
end

return M
