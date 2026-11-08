package data

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// AgentConfigFile is agent.toml, beside lanes.toml in tern-kata's config folder. The user writes it by hand; the Pane never does.
//
//	actor = "me-agent"
//	name = "builder"
//	command = ["omp"]
//	forward_comments = true
//	message_from = ["dana", "dana-laptop"]
//	[projects.tern-kata]
//	actor = "..."
//	name = "..."
//	command = ["omp", "--model", "opus"]
//	checkout = "/home/me/Code/tern-kata"
const AgentConfigFile = "agent.toml"

// DefaultAgentCommand is the agent program when agent.toml names none.
var DefaultAgentCommand = []string{worker.DefaultCommand}

// AgentConfig is agent.toml, strictly decoded.
type AgentConfig struct {
	Actor   string   `toml:"actor"`
	Name    string   `toml:"name"`
	Command []string `toml:"command"`
	// ForwardComments turns on telling the agent about a person's comment on its Issue; unset is off.
	ForwardComments *bool `toml:"forward_comments"`
	// MessageFrom lists the actors, besides the agent's own, whose messages the agent acts on; unset and empty are none.
	MessageFrom []string                `toml:"message_from"`
	Projects    map[string]AgentProject `toml:"projects"`
}

// AgentProject is one [projects.<name>] table; its values override the top-level ones.
type AgentProject struct {
	Actor    string   `toml:"actor"`
	Name     string   `toml:"name"`
	Command  []string `toml:"command"`
	Checkout string   `toml:"checkout"`
	// ForwardComments overrides the top-level value when set.
	ForwardComments *bool `toml:"forward_comments"`
	// MessageFrom overrides the top-level list when set, an empty list included.
	MessageFrom []string `toml:"message_from"`
}

// ReadAgentConfig reads and validates path; a missing file is an empty configuration. Any other failure names the file.
func ReadAgentConfig(path string) (AgentConfig, error) {
	var c AgentConfig
	meta, err := toml.DecodeFile(path, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return AgentConfig{}, nil
	}
	if err != nil {
		return AgentConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return AgentConfig{}, fmt.Errorf("%s: unknown key %s", path, strings.Join(keys, ", "))
	}
	if meta.IsDefined("name") {
		if err := checkTeammate(c.Name); err != nil {
			return AgentConfig{}, fmt.Errorf("%s: name: %w", path, err)
		}
	}
	if meta.IsDefined("command") {
		if err := checkAgentCommand(c.Command); err != nil {
			return AgentConfig{}, fmt.Errorf("%s: command: %w", path, err)
		}
	}
	if err := checkMessageFrom(c.MessageFrom); err != nil {
		return AgentConfig{}, fmt.Errorf("%s: message_from: %w", path, err)
	}
	for _, name := range slices.Sorted(maps.Keys(c.Projects)) {
		p := c.Projects[name]
		if meta.IsDefined("projects", name, "name") {
			if err := checkTeammate(p.Name); err != nil {
				return AgentConfig{}, fmt.Errorf("%s: projects.%s.name: %w", path, name, err)
			}
		}
		if meta.IsDefined("projects", name, "command") {
			if err := checkAgentCommand(p.Command); err != nil {
				return AgentConfig{}, fmt.Errorf("%s: projects.%s.command: %w", path, name, err)
			}
		}
		if meta.IsDefined("projects", name, "checkout") && !filepath.IsAbs(p.Checkout) {
			return AgentConfig{}, fmt.Errorf("%s: projects.%s.checkout: %q is not an absolute directory", path, name, p.Checkout)
		}
		if err := checkMessageFrom(p.MessageFrom); err != nil {
			return AgentConfig{}, fmt.Errorf("%s: projects.%s.message_from: %w", path, name, err)
		}
	}
	return c, nil
}

// checkMessageFrom refuses an actor that is empty or holds a comma, which TERN_KATA_MESSAGE_FROM uses to separate them.
func checkMessageFrom(actors []string) error {
	for _, actor := range actors {
		if strings.TrimSpace(actor) == "" || strings.ContainsAny(actor, ", \t\r\n") {
			return fmt.Errorf("%q is not an actor (not empty, no commas or spaces)", actor)
		}
	}
	return nil
}

// checkTeammate refuses a name that is no teammate name; the rule is model.ValidTeammate's.
func checkTeammate(name string) error {
	if !model.ValidTeammate(name) {
		return fmt.Errorf("%q is not a teammate name (letters, digits, . _ -; at most 64)", name)
	}
	return nil
}

func checkAgentCommand(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return errors.New("must name a program")
	}
	return nil
}

// AgentSettings are the actor, command and checkout an agent on a Project starts with.
type AgentSettings struct {
	// Actor is "" when agents run as whoever kata resolves in the agent's shell.
	Actor string
	// Name is the teammate the agent's Attention requests carry: the configured name, else the command's program name, "" when that is no valid teammate.
	Name    string
	Command []string
	// Checkout is the Project's table's checkout, "" when it has none.
	Checkout string
	// ForwardComments is whether the agent is told of a person's comment on its Issue.
	ForwardComments bool
	// MessageFrom is the actors, besides Actor, whose messages the agent acts on.
	MessageFrom []string
}

// For is the settings for a Project: its table's values over the top-level ones over the defaults.
func (c AgentConfig) For(project string) AgentSettings {
	s := AgentSettings{Actor: c.Actor, Name: c.Name, Command: c.Command, ForwardComments: c.ForwardComments != nil && *c.ForwardComments, MessageFrom: c.MessageFrom}
	if p, ok := c.Projects[project]; ok {
		if p.Actor != "" {
			s.Actor = p.Actor
		}
		if p.Name != "" {
			s.Name = p.Name
		}
		if len(p.Command) > 0 {
			s.Command = p.Command
		}
		s.Checkout = p.Checkout
		if p.ForwardComments != nil {
			s.ForwardComments = *p.ForwardComments
		}
		if p.MessageFrom != nil {
			s.MessageFrom = p.MessageFrom
		}
	}
	if len(s.Command) == 0 {
		s.Command = DefaultAgentCommand
	}
	if s.Name == "" {
		program := filepath.Base(s.Command[0])
		if name := strings.TrimSuffix(program, filepath.Ext(program)); model.ValidTeammate(name) {
			s.Name = name
		}
	}
	return s
}

// AgentDir is the directory an agent starts in, first match winning: the Binding's workspace (binding, non-empty only when the
// Issue's Project is the Binding), agent.toml's checkout for the Project (settings), the first checkout worker.toml lists for
// the Project at workerConfigPath. None is ErrNoAgentDir. A worker.toml that is missing or does not load has no checkouts.
func AgentDir(binding, project string, settings AgentSettings, workerConfigPath string) (string, error) {
	if binding != "" {
		return binding, nil
	}
	if settings.Checkout != "" {
		return settings.Checkout, nil
	}
	if cfg, err := worker.LoadConfig(workerConfigPath); err == nil {
		for _, p := range cfg.Projects {
			if p.Name == project && len(p.Checkouts) > 0 {
				return cfg.Checkouts[p.Checkouts[0]], nil
			}
		}
	}
	return "", fmt.Errorf("no checkout is known for Project %q; add [projects.%s] checkout to %s", project, project, AgentConfigFile)
}
