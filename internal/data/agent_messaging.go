package data

import (
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data/ompext"
	"github.com/Dangerboi/tern-kata/internal/model"
)

const OmpExtensionFolder = "omp"

// AgentMessaging is how an agent hears the messages written to it: AddressPrefix is its Agent address up to the block id
// (<actor>/<name>), which the wrapper completes from the block it runs in, and ExtensionArgs the omp flag and path (-e <path>) that load the
// extension delivering them (ompext.Argv). ForwardComments turns on the extension's delivery of a person's comment on the agent's Issue.
// MessageFrom is the actors, besides the agent's own, whose messages it acts on. The zero value is an agent no message reaches.
type AgentMessaging struct {
	AddressPrefix   string
	ExtensionArgs   []string
	ForwardComments bool
	MessageFrom     []string
	// Coordinator is the Agent address of the coordinator of the team this agent is a child of; Coordinating marks the coordinator
	// itself. The wrapper turns them into TERN_KATA_COORDINATOR and TERN_KATA_COORDINATING=1, which the extension reads.
	Coordinator  string
	Coordinating bool
}

// AgentAddress is the Agent address of the agent in block whose address prefix is prefix: the one spelling the Pane records and
// the wrapper sets as KATA_INBOX_USER.
func AgentAddress(prefix, block string) string { return prefix + "-" + block }

func AddressActor(address string) string {
	if i := strings.LastIndex(address, "/"); i > 0 {
		return address[:i]
	}
	return ""
}

// agentNoAddress says why an agent started with command cannot load tern-kata's omp extension, so has no Agent address, "" when
// it can: the program must be omp.
func agentNoAddress(command []string) model.AgentNoAddress {
	if !ompext.IsOmp(command[0]) {
		return model.AgentNotOmp
	}
	return ""
}
