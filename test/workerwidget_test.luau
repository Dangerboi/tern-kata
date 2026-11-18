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

-- The clock the widget is rendered at, and a held run with the fields a test does not name defaulted.
local NOW = 1800000000
local function held(fields: { [string]: any }): workersummary.Held
    return {
        uid = fields.uid or "u", job = fields.job or "j", ref = fields.ref or "k#a1", reason = fields.reason or "r",
        resumable = fields.resumable == true, abandonable = fields.abandonable == true, heldAt = fields.heldAt,
    }
end

local function heldRuns(count: number): { workersummary.Held }
    local list: { workersummary.Held } = {}
    for index = 1, count do table.insert(list, held({uid = "run" .. index, job = "job" .. index, ref = "r" .. index, reason = "why" .. index, resumable = true})) end
    return list
end

local function view(s: workersummary.Summary?, onHost: boolean?): WidgetView?
    local current = s
    local spec = workerwidget.spec("tern-kata", if onHost == nil then true else onHost, function() return current end, function() return NOW end)
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
            held({job = "j", ref = "algonaut-trading-platform#a1b2c3d4", reason = "secret_unavailable: the token for the project is gone"}),
            held({job = "j", ref = "r", reason = "agent_quota: " .. string.rep("w", 90)}),
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

test("the card lists each held run as its job and age, Issue link and Resume over its reason, then what runs and which worker", function()
    local rows = cardRows(summary(2, {held({uid = "01RUN", job = "review", ref = "kata-a#a1b2", reason = "agent_quota: out of tokens", resumable = true, heldAt = NOW - 7300})}))
    assert(#rows == 4, #rows)
    local head = rows[1]
    assert(head.label == "review · 2 h" and head.text == "kata-a#a1b2" and head.href == "https://kata.invalid/kata-a/a1b2", head.label)
    assert(head.id == "01RUN" and head.tone == nil and head.actions ~= nil and #head.actions == 1 and head.actions[1].id == workerwidget.RESUME_ID)
    assert(rows[2].label == nil and rows[2].text == "agent_quota: out of tokens" and rows[2].tone == "warn")
    assert(rows[3].label == "Running" and rows[3].text == "2")
    assert(rows[4].label == "Worker" and rows[4].text == "box · pid 42", rows[4].text)
end)

test("the Worker row says this machine on the window half's widget, and leaves out an executor the worker does not name", function()
    assert(cardRows(summary(0), false)[2].text == "box · this machine · pid 42")
    local nameless = summary(0)
    nameless.executor = ""
    assert(cardRows(nameless)[2].text == "pid 42")
end)

test("a held run's age is in its largest whole unit, and a run without a held time shows none", function()
    assert(workerwidget.age(NOW - 59, NOW) == "<1 m" and workerwidget.age(NOW + 30, NOW) == "<1 m")
    assert(workerwidget.age(NOW - 60, NOW) == "1 m" and workerwidget.age(NOW - 3599, NOW) == "59 m")
    assert(workerwidget.age(NOW - 3600, NOW) == "1 h" and workerwidget.age(NOW - 86399, NOW) == "23 h")
    assert(workerwidget.age(NOW - 3 * 86400, NOW) == "3 d")
    assert(cardRows(summary(0, {held({job = "review"})}))[1].label == "review")
    assert(cardRows(summary(0, {held({job = "", heldAt = NOW - 120})}))[1].label == "held · 2 m")
end)

test("the clock key changes each minute while runs are held, and never without any", function()
    local some = summary(0, heldRuns(1))
    assert(workerwidget.clockKey(some, NOW) ~= workerwidget.clockKey(some, NOW + 60))
    assert(workerwidget.clockKey(some, 120) == workerwidget.clockKey(some, 179))
    assert(workerwidget.clockKey(summary(3), NOW) == workerwidget.clockKey(summary(3), NOW + 3600))
    assert(workerwidget.clockKey(nil, NOW) == "")
end)

test("a long reason wraps by words on up to three full-width rows under its run, the last ending in an ellipsis", function()
    local reason = "agent_quota: the agent ran out of quota for this billing period; resume it once the quota resets at midnight, or switch the job to another agent"
    local rows = cardRows(summary(0, {held({reason = reason})}))
    assert(#rows == 6, #rows)
    assert(rows[2].text == "agent_quota: the agent ran out of quota for", rows[2].text)
    assert(rows[3].text == "this billing period; resume it once the", rows[3].text)
    assert(rows[4].text == "quota resets at midnight, or switch the job…" and rows[4].label == nil and rows[4].tone == "warn", rows[4].text)
    for index = 2, 4 do assert((utf8.len(rows[index].text) :: number) <= 44, rows[index].text) end
end)

test("a run the worker would resume offers Resume; one it would only abandon offers Abandon on an error-toned row; else none", function()
    local rows = cardRows(summary(0, {
        held({uid = "a", resumable = true, abandonable = true}),
        held({uid = "b", abandonable = true}),
        held({uid = "c"}),
    }))
    assert(assert(rows[1].actions)[1].id == workerwidget.RESUME_ID and rows[1].tone == nil)
    local abandon = assert(rows[3].actions)[1]
    assert(rows[3].id == "b" and rows[3].tone == "error" and abandon.id == workerwidget.ABANDON_ID and abandon.label == "Abandon")
    assert(rows[5].actions == nil and rows[5].id == nil and rows[5].tone == nil)
end)

test("only the session host's half offers buttons, and only for a uid Tern takes as a row id", function()
    local runs = {held({uid = "a", resumable = true}), held({uid = "", resumable = true}), held({uid = string.rep("u", 65), abandonable = true})}
    local rows = cardRows(summary(0, runs))
    assert(rows[1].actions ~= nil)
    for _, index in {3, 5} do assert(rows[index].actions == nil and rows[index].id == nil, index) end
    for _, row in cardRows(summary(0, runs), false) do assert(row.actions == nil) end
end)

test("a run with no job, Issue or reason keeps its heading row, and a ref the link route cannot carry gets no link", function()
    local rows = cardRows(summary(0, {
        held({job = "", ref = "my project#a1"}),
        held({job = "", ref = "", reason = ""}),
        held({job = string.rep("j", 30), ref = "odd;$x&y#c1"}),
    }))
    assert(rows[1].href == nil and rows[1].label == "held" and rows[1].text == "my project#a1")
    assert(rows[3].href == nil and rows[3].text == "no Issue" and rows[4].label == string.rep("j", 15) .. "…", rows[4].label)
    assert(rows[4].href == "https://kata.invalid/odd;$x&y/c1", rows[4].href)
end)

test("the card keeps to Tern's 32 rows: held runs that do not fit become one row saying how many more", function()
    local rows = cardRows(summary(1, heldRuns(40)))
    assert(#rows == 31, #rows)
    assert(rows[27].text == "r14" and rows[28].text == "why14" and rows[29].text == "and 26 more held", rows[29].text)
    assert(rows[30].label == "Running" and rows[31].label == "Worker")
    local fifteen = cardRows(summary(0, heldRuns(15)))
    assert(#fifteen == 32 and fifteen[30].text == "why15")
end)

harness.finish()
