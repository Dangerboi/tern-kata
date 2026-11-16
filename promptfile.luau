--!strict

-- The one rule for naming a tern-kata agent's prompt file in text (a command line, a tab's running line): the whole path
-- `agent-prompts/prompt-<random>.md` or `agent-prompts/prompt-<starter>-<random>.md`, with either slash. It is not followed by a
-- name character or a dot, so `<prompt>.md.token` (a secret file beside it) is another file. The path may stand as the `@<path>`
-- word omp reads or as the wrapper's `--prompt-file=<path>` flag, which a reopened chat's line keeps. Go (internal/model
-- promptTail) and the omp extension (promptFileFromArgv, which reads omp's own `@` words) apply the same rule.

local M = {}

-- Where the first prompt file in `text` starts and ends, nil when there is none.
function M.span(text: string?): (number?, number?)
    if type(text) ~= "string" then return nil, nil end
    local init = 1
    while true do
        local s, e = text:find("agent%-prompts[/\\]prompt%-[%w_%-]+%.md", init)
        if s == nil or e == nil then return nil, nil end
        local after = text:sub(e + 1, e + 1)
        if after == "" or not after:find("[%w_.]") then return s, e end
        init = e + 1
    end
end

function M.has(text: string?): boolean
    return (M.span(text)) ~= nil
end

-- The block of the kata Pane that started the agent, which the file's name carries; nil for a split agent's name.
function M.starter(text: string?): number?
    local s, e = M.span(text)
    if s == nil or e == nil or type(text) ~= "string" then return nil end
    local block = text:sub(s, e):match("prompt%-(%d+)%-[%w_%-]+%.md$")
    return block and tonumber(block) or nil
end

-- Whether `path` is a prompt file and nothing else.
function M.isPath(path: string): boolean
    local s, e = M.span(path)
    return s ~= nil and e == #path
end

return M
