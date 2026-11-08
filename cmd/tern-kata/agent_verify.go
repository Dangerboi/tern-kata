package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
)

// agentVerifyDeadline bounds the wrapper's one read of the daemon before it starts the agent.
const agentVerifyDeadline = 10 * time.Second

// verifyAgentRoute asks the daemon on the route environ names (as routedEnv left it) whether it verifies the actor of every write,
// which the agent's extension needs before it acts on a message_from actor's message. A daemon that cannot be reached or does not
// answer reads as not verified: the agent still starts, and acts on its own actor's messages only.
func verifyAgentRoute(environ []string) bool {
	// kata's discovery reads the process environment, so the route must be in it; the agent's own environment is built from environ.
	os.Clearenv()
	for _, kv := range environ {
		if name, value, ok := strings.Cut(kv, "="); ok && name != "" {
			_ = os.Setenv(name, value)
		}
	}
	workspace, err := os.Getwd()
	if err != nil {
		return false
	}
	return daemonVerifiesSenders(context.Background(), data.LocateWithKata, workspace)
}

// daemonVerifiesSenders is whether the daemon locate finds for workspace verifies actors, "no" on any failure.
func daemonVerifiesSenders(ctx context.Context, locate data.Locator, workspace string) bool {
	ctx, cancel := context.WithTimeout(ctx, agentVerifyDeadline)
	defer cancel()
	verified, err := data.ReadVerifiedSenders(ctx, locate, workspace)
	return err == nil && verified
}
