package data

// AgentConfigStarter is what the Config Page's e opens when there is no agent.toml: every line a comment, so saving it unchanged
// is no change, and each key AgentConfig and AgentProject accept shown once where it goes. A test keeps it in step with those
// structs, so a key added to the parser cannot be left out of it.
const AgentConfigStarter = `# agent.toml: how an agent started from the Agent card runs. Every key is optional; take the # off a line to use it.
# It is read on every agent start, so a saved change applies to agents started from then on; running agents keep their settings.

# The actor the agent writes to kata as. Without one it is whoever kata resolves in the agent's shell.
# actor = "me-agent"

# This machine's name on the Agent records it publishes. Top level only; default is the machine's host name.
# host = "laptop"

# The teammate name the agent's Attention requests carry. Default is the command's program name.
# name = "builder"

# The agent program and its arguments; the Pane appends @<prompt file>, so it must accept omp's @file message.
# command = ["omp"]

# Tell the agent about a person's comment on its Issue. Default is false.
# forward_comments = true

# Actors, besides the agent's own, whose messages the agent acts on. No commas or spaces in an actor.
# message_from = ["dana", "dana-laptop"]

# A table per Project, by Project name. Its values override the top-level ones above.
# [projects.tern-kata]
# actor = "me-agent"
# name = "builder"
# command = ["omp", "--model", "opus"]
# forward_comments = false
# message_from = []
# The directory the agent starts in, an absolute path.
# checkout = "/home/me/Code/tern-kata"
`
