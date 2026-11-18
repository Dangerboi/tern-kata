--!strict

-- Tests of what the kata worker widget shows, on plain data, outside Tern: `luau test/workerwidget_test.luau`.

local workersummary = require("../workersummary")
local workerwidget = require("../workerwidget")
local harness = require("./harness")

local test = harness.test

local HELD = "plugin.tern-kata.show-held-runs"
local OPEN = "plugin.tern-kata.open"

local function summary(running: number, held: { workersummary.Held }?, capacity: number?, inUse: number?): workersummary.Summary
    return {executor = "box", pid = 42, running = running, capacity = capacity or 4, inUse = inUse or running, held = held or {}}
end

local function heldRuns(count: number): { workersummary.Held }
    local list: { workersummary.Held } = {}
    for index = 1, count do table.insert(list, {uid = "run" .. index, job = "job" .. index, ref = "r" .. index, reason = "why" .. index, resumable = true}) end
    return list
end

local function view(s: workersummary.Summary?, onHost: boolean?): WidgetView?
    local current = s
    local spec = workerwidget.spec("tern-kata", if onHost == nil then true else onHost, function() return current end)
    return spec.render(nil)
end

local function shown(s: workersummary.Summary?, onHost: boolean?): WidgetView
    return assert(view(s, onHost))
end

local function titleLines(s: workersummary.Summary, onHost: boolean?): { string }
    return (shown(s, onHost).title :: string):split("\n")
end

local function cardRows(s: workersummary.Summary, onHost: boolean?): { WidgetCardRow }
    return assert(shown(s, onHost).card)
end

test("no summary shows nothing", function()
    assert(view(nil, true) == nil)
end)

test("nothing running is idle, muted, and opens kata", function()
    local v = shown(summary(0))
    assert(v.text == "idle" and v.tone == "muted" and v.command == OPEN)
end)

test("running runs show muted and open kata", function()
    local v = shown(summary(3))
    assert(v.text == "3 running" and v.tone == "muted" and v.command == OPEN)
end)

test("held runs show the warning tone and the held command", function()
    local v = shown(summary(1, heldRuns(2)))
    assert(v.text == "2 held" and v.tone == "warning" and v.command == HELD)
end)

test("value is in use over capacity, and 0 for no capacity", function()
    assert(shown(summary(1, nil, 4, 1)).value == 0.25)
    assert(shown(summary(0, nil, 0, 0)).value == 0)
end)

test("the title's first line names the session host's worker when the host draws it, this machine's when the window half does", function()
    assert(titleLines(summary(0), true)[1] == "Worker on the session host")
    assert(titleLines(summary(0), false)[1] == "Worker on this machine")
end)

test("host and fallback views differ only in the title's place", function()
    local host, fallback = shown(summary(1, heldRuns(1)), true), shown(summary(1, heldRuns(1)), false)
    assert(host.text == fallback.text and host.tone == fallback.tone and host.command == fallback.command and host.value == fallback.value)
    assert(host.title ~= fallback.title)
end)

test("the title lists up to five held runs then how many more, then the running count, process and executor", function()
    local lines = titleLines(summary(2, heldRuns(7)))
    assert(#lines == 8, #lines)
    assert(lines[2] == "why1 r1" and lines[6] == "why5 r5")
    assert(lines[7] == "and 2 more" and lines[8] == "2 running, process 42, box")
end)

test("five held runs get no more line", function()
    local lines = titleLines(summary(0, heldRuns(5)))
    assert(#lines == 7 and lines[7] == "0 running, process 42, box")
end)

test("every title line fits Tern's width for a long executor, Issue ref and reason", function()
    local long = {
        executor = string.rep("e", 80), pid = 4242, running = 3, capacity = 4, inUse = 3,
        held = {
            {uid = "a", job = "j", ref = "algonaut-trading-platform#a1b2c3d4", reason = "secret_unavailable: the token for the project is gone", resumable = false},
            {uid = "b", job = "j", ref = "r", reason = "agent_quota: " .. string.rep("w", 90), resumable = false},
        },
    }
    for _, onHost in {true, false} do
        for _, line in titleLines(long, onHost) do
            assert((utf8.len(line) or #line) <= 45, line)
        end
    end
    local lines = titleLines(long)
    assert(lines[1] == "Worker on the session host")
    assert(lines[2]:sub(1, 19) == "secret_unavailable:" and lines[3]:sub(1, 13) == "agent_quota: ")
    assert(lines[4]:sub(1, 29) == "3 running, process 4242, eeee", lines[4])
end)

test("the card says where the worker is, each held run as its job, Issue link and Resume over its reason, then what runs and the process", function()
    local held: { workersummary.Held } = {{uid = "01RUN", job = "review", ref = "kata-a#a1b2", reason = "agent_quota: out of tokens", resumable = true}}
    local rows = cardRows(summary(2, held))
    assert(#rows == 5, #rows)
    assert(rows[1].label == "Worker" and rows[1].text == "on the session host")
    local head = rows[2]
    assert(head.label == "review" and head.text == "kata-a#a1b2" and head.href == "https://kata.invalid/kata-a/a1b2", head.href)
    assert(head.id == "01RUN" and head.actions ~= nil and #head.actions == 1 and head.actions[1].id == workerwidget.RESUME_ID)
    assert(rows[3].label == nil and rows[3].text == "agent_quota: out of tokens" and rows[3].tone == "warn")
    assert(rows[4].label == "Running" and rows[4].text == "2")
    assert(rows[5].label == "Process" and rows[5].text == "42 on box")
    assert(cardRows(summary(0), false)[1].text == "on this machine")
end)

test("a long reason wraps by words on up to three full-width rows under its run, the last ending in an ellipsis", function()
    local reason = "agent_quota: the agent ran out of quota for this billing period; resume it once the quota resets at midnight, or switch the job to another agent"
    local held: { workersummary.Held } = {{uid = "a", job = "review", ref = "k#a1", reason = reason, resumable = false}}
    local rows = cardRows(summary(0, held))
    assert(#rows == 7, #rows)
    assert(rows[3].text == "agent_quota: the agent ran out of quota for", rows[3].text)
    assert(rows[4].text == "this billing period; resume it once the", rows[4].text)
    assert(rows[5].text == "quota resets at midnight, or switch the job…" and rows[5].label == nil and rows[5].tone == "warn", rows[5].text)
    for index = 3, 5 do assert((utf8.len(rows[index].text) :: number) <= 44, rows[index].text) end
end)

test("only the session host's half offers Resume, and only for a run the worker would resume", function()
    local held: { workersummary.Held } = {
        {uid = "a", job = "j", ref = "k#a1", reason = "r", resumable = true},
        {uid = "b", job = "j", ref = "k#b1", reason = "r", resumable = false},
        {uid = "", job = "j", ref = "k#c1", reason = "r", resumable = true},
        {uid = string.rep("u", 65), job = "j", ref = "k#d1", reason = "r", resumable = true},
    }
    local rows = cardRows(summary(0, held))
    assert(rows[2].actions ~= nil)
    for _, index in {4, 6, 8} do assert(rows[index].text:sub(1, 2) == "k#" and rows[index].actions == nil and rows[index].id == nil, index) end
    assert(cardRows(summary(0, held), false)[2].actions == nil)
end)

test("a run with no job, Issue or reason keeps its heading row, and a ref the link route cannot carry gets no link", function()
    local held: { workersummary.Held } = {
        {uid = "a", job = "", ref = "my project#a1", reason = "r", resumable = false},
        {uid = "b", job = "", ref = "", reason = "", resumable = false},
        {uid = "c", job = string.rep("j", 30), ref = "odd;$x&y#c1", reason = "r", resumable = false},
    }
    local rows = cardRows(summary(0, held))
    assert(rows[2].href == nil and rows[2].label == nil and rows[2].text == "my project#a1")
    assert(rows[4].href == nil and rows[4].text == "held run" and rows[5].label == "j" .. string.rep("j", 18) .. "…", rows[5].label)
    assert(rows[5].href == "https://kata.invalid/odd;$x&y/c1", rows[5].href)
end)

test("the card keeps to Tern's 32 rows: held runs that do not fit become one row saying how many more", function()
    local rows = cardRows(summary(1, heldRuns(40)))
    assert(#rows == 32, #rows)
    assert(rows[28].text == "r14" and rows[29].text == "why14" and rows[30].text == "and 26 more held", rows[30].text)
    assert(rows[31].label == "Running" and rows[32].label == "Process")
    local fourteen = cardRows(summary(0, heldRuns(14)))
    assert(#fourteen == 31 and fourteen[29].text == "why14")
end)

harness.finish()
