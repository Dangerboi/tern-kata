--!strict

-- Tests of what a waiting agent's Attention request says, on plain data, outside Tern: `luau test/waitingtext_test.luau`.
-- The tool and message shapes are what Tern 0.9.9 returns for an omp agent waiting on its ask tool.

local waitingtext = require("../waitingtext")

local failures = 0
local function test(name: string, body: () -> ())
    local ok, err = (pcall :: any)(body)
    if ok then
        print("ok   " .. name)
    else
        failures += 1
        print("FAIL " .. name .. ": " .. tostring(err))
    end
end

local function ask(question: string, status: string?): { [string]: any }
    return {name = "ask", role = "omp.tool.ask", status = status or "running", target = question, text = "Waiting for your answer…"}
end

local function said(text: string, tools: { { [string]: any } }?): waitingtext.Message
    return {role = "assistant", text = text, tools = tools or {}}
end

local function asked(text: string): waitingtext.Message
    return {role = "user", text = text, tools = {}}
end

local thinking = "Thinking…\nI'm checking who owns the claim on"

test("an agent waiting on its ask tool gets the question, not the thinking text", function()
    local got = waitingtext.of({asked("pick"), said("", {ask("Which colour should be used?")})}, thinking)
    assert(got == "Which colour should be used?", got)
end)

test("an ask call beats the assistant text of the same message", function()
    local got = waitingtext.of({said("Let me ask.", {ask("Which size?")})}, thinking)
    assert(got == "Which size?", got)
end)

test("the pending ask is the last running one, other tools are skipped", function()
    local got = waitingtext.of({said("", {ask("Old?", "done"), {name = "read", status = "done", target = "a.go"}, ask("New?")})}, "")
    assert(got == "New?", got)
end)

test("an answered ask is not the request", function()
    local got = waitingtext.of({said("Done, anything else?", {ask("Old?", "done")})}, thinking)
    assert(got == "Done, anything else?", got)
end)

test("a plain assistant message is the text", function()
    local got = waitingtext.of({asked("fix it"), said("Shall I run the tests?")}, thinking)
    assert(got == "Shall I run the tests?", got)
end)

test("only the last assistant message counts", function()
    local got = waitingtext.of({said("", {ask("Old?")}), asked("red"), said("All set.")}, thinking)
    assert(got == "All set.", got)
end)

test("a trailing user message is passed over", function()
    local got = waitingtext.of({said("Ready?"), asked("yes")}, thinking)
    assert(got == "Ready?", got)
end)

test("an omp agent with no usable text gets the empty text, never the thinking text", function()
    assert(waitingtext.of({asked("go"), said("")}, thinking) == "")
    assert(waitingtext.of({said("  \n", {ask("   ")})}, thinking) == "")
    assert(waitingtext.of({asked("go")}, thinking) == "")
end)

test("an empty transcript keeps the last message, as for an agent that is not omp", function()
    assert(waitingtext.of({}, "Which one?") == "Which one?")
    assert(waitingtext.of({}, "") == "")
end)

if failures > 0 then
    print(failures .. " failed")
    error("failures")
end
print("all passed")
