--!strict

-- What an agent that waits for you is waiting on, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/waitingtext_test.luau`. window.luau reads the agent's transcript, asks `of` and relays the answer as the
-- agent's last message, which the Pane writes into the Attention request.

export type Message = { role: "user" | "assistant", text: string, tools: { { [string]: any } } }

local M = {}

local function usable(text: any): boolean
    return type(text) == "string" and text:match("%S") ~= nil
end

-- The question of the ask call the agent still waits on: Tern's omp transcript keeps the call as a tool named "ask", status
-- "running" until it is answered, whose `target` is the question (the first one when the call asks several).
local function pendingQuestion(tools: { { [string]: any } }): string?
    for i = #tools, 1, -1 do
        local tool = tools[i]
        if tool.name == "ask" and tool.status == "running" and usable(tool.target) then return tool.target end
    end
    return nil
end

-- The text for the Attention request of a waiting agent. `transcript` is the end of `cx.agents:transcript`, which is empty for
-- an agent that is not omp: then `lastMessage` (Tern's last assistant text) is what there is. For omp the last assistant
-- message decides, and the thinking text that `lastMessage` can hold is never used: the pending ask's question, else the
-- message's text, else "" (the Pane then says "agent waiting for you").
function M.of(transcript: { Message }, lastMessage: string): string
    if #transcript == 0 then return lastMessage end
    for i = #transcript, 1, -1 do
        local message = transcript[i]
        if message.role == "assistant" then
            return pendingQuestion(message.tools) or (if usable(message.text) then message.text else "")
        end
    end
    return ""
end

return M
