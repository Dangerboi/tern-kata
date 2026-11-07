package data

import (
	"github.com/Dangerboi/tern-kata/internal/data/ompext"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// OmpExtensionFolder is the folder of tern-kata's state folder that holds the omp extension the agents load.
const OmpExtensionFolder = "omp"

// AgentMessaging is how an agent hears the messages written to it: AddressPrefix is its Agent address up to the block id
// (<actor>/<name>), which the wrapper completes from the block it runs in, and ExtensionArgs the omp flag and path (-e <path>) that load the
// extension delivering them (ompext.Argv). ForwardComments turns on the extension's delivery of a person's comment on the agent's Issue. The zero value is an agent no message reaches.
type AgentMessaging struct {
	AddressPrefix   string
	ExtensionArgs   []string
	ForwardComments bool
}

// AgentAddress is the Agent address of the agent in block whose address prefix is prefix: the one spelling the Pane records and
// the wrapper sets as KATA_INBOX_USER.
func AgentAddress(prefix, block string) string { return prefix + "-" + block }

// agentNoAddress says why an agent started with command cannot load tern-kata's omp extension, so has no Agent address, "" when
// it can: the program must be omp.
func agentNoAddress(command []string) model.AgentNoAddress {
	if !ompext.IsOmp(command[0]) {
		return model.AgentNotOmp
	}
	return ""
}
