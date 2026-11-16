--!strict

-- Tests of the rule that names an agent's prompt file in a command line, outside Tern: `luau test/promptfile_test.luau`.

local promptfile = require("../promptfile")
local harness = require("./harness")

local test = harness.test

test("a tab's or background agent's file names the starting Pane's block", function()
    assert(promptfile.starter("sleep @/state/agent-prompts/prompt-12-345.md") == 12)
    assert(promptfile.starter("omp @C:\\state\\agent-prompts\\prompt-7-1.md") == 7)
end)

test("a split agent's file is a prompt file with no starter", function()
    assert(promptfile.has("omp @/state/agent-prompts/prompt-345.md"))
    assert(promptfile.starter("omp @/state/agent-prompts/prompt-345.md") == nil)
end)

test("a secret file beside the prompt file is not the prompt file", function()
    assert(not promptfile.has("omp --token-file=/state/agent-prompts/prompt-12-345.md.token"))
    assert(not promptfile.has("omp --env-file /state/agent-prompts/prompt-12-345.md.env"))
    assert(promptfile.has("omp --token-file=/state/agent-prompts/prompt-12-345.md.token @/state/agent-prompts/prompt-12-345.md"))
end)

test("the wrapper's --prompt-file flag names the prompt file, in either form", function()
    assert(promptfile.has("tern-kata agent --prompt-file=/state/agent-prompts/prompt-12-345.md --ref=p#1 -- omp"))
    assert(promptfile.has("tern-kata agent --prompt-file /state/agent-prompts/prompt-12-345.md -- omp"))
    assert(promptfile.has("tern-kata agent '--prompt-file=C:\\state\\agent-prompts\\prompt-12-345.md' -- omp"))
    assert(promptfile.starter("tern-kata agent --prompt-file=/state/agent-prompts/prompt-12-345.md -- omp") == 12)
    assert(promptfile.has("tern-kata agent --prompt-file=/state/agent-prompts/prompt-345.md"))
end)

test("a file outside the agent-prompts folder, or not named prompt-*.md, is not one", function()
    assert(not promptfile.has("omp @/state/prompt-12-345.md"))
    assert(not promptfile.has("omp @/state/agent-prompts/notes.md"))
    assert(not promptfile.has(nil))
    assert(not promptfile.isPath("@spec.md"))
    assert(promptfile.isPath("/s/agent-prompts/prompt-1-2.md"))
    assert(not promptfile.isPath("/s/agent-prompts/prompt-1-2.md.token"))
end)

harness.finish()
