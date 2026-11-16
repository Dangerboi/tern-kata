--!strict

-- Shared runner for the Luau tests: `test` runs one case, `finish` fails the file if any case failed.

local failures = 0

local harness = {}

function harness.test(name: string, body: () -> ())
    local ok, err = (pcall :: any)(body)
    if ok then
        print("ok   " .. name)
    else
        failures += 1
        print("FAIL " .. name .. ": " .. tostring(err))
    end
end

function harness.finish()
    if failures > 0 then error(failures .. " test(s) failed", 0) end
end

return harness
