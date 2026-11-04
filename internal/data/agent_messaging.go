package data

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// ompExtension is tern-kata's omp extension: in an omp agent it follows kata's event stream and delivers the messages written to
// the agent's own Agent address as user messages. Its source and Bun tests live in ompext/.
//
//go:embed ompext/tern-kata.ts
var ompExtension []byte

// OmpExtensionFolder is the folder of tern-kata's state folder that holds the omp extension the agents load.
const OmpExtensionFolder = "omp"

// AgentMessaging is how an agent hears the messages written to it: AddressPrefix is its Agent address up to the block id
// (<actor>/<name>), which the wrapper completes from the block it runs in, and Extension the omp extension that delivers them. The
// zero value is an agent no message reaches.
type AgentMessaging struct{ AddressPrefix, Extension string }

// AgentAddress is the Agent address of the agent in block whose address prefix is prefix: the one spelling the Pane records and
// the wrapper sets as KATA_INBOX_USER.
func AgentAddress(prefix, block string) string { return prefix + "-" + block }

// agentNoAddress says why an agent started with command cannot load tern-kata's omp extension, so has no Agent address, "" when
// it can: the program must be omp, and omp refuses -e beside --trusted-extension.
func agentNoAddress(command []string) model.AgentNoAddress {
	program := filepath.Base(command[0])
	if strings.TrimSuffix(program, filepath.Ext(program)) != worker.DefaultCommand {
		return model.AgentNotOmp
	}
	if slices.ContainsFunc(command[1:], func(arg string) bool {
		return arg == "--trusted-extension" || strings.HasPrefix(arg, "--trusted-extension=")
	}) {
		return model.AgentTrustedExtension
	}
	return ""
}

// writeOmpExtension puts the omp extension in the state folder, replacing a file of other content (an older tern-kata's), and
// returns its path. An agent already running keeps the extension it loaded.
func writeOmpExtension(state string) (string, error) {
	path := filepath.Join(state, OmpExtensionFolder, "tern-kata.ts")
	if held, err := os.ReadFile(path); err == nil && bytes.Equal(held, ompExtension) {
		return path, nil
	}
	if err := atomicfile.Write(path, ompExtension, atomicfile.Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		return "", err
	}
	return path, nil
}
