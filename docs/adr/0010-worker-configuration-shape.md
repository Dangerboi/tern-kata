# The worker's config is one strict TOML file that names what it may serve and holds no secret values

`worker.toml`, in tern-kata's user config folder beside `lanes.toml`, lists the Projects the worker serves and the local things jobs may name: checkouts, agent profiles, secret sources and limits. The worker reads it at start only; a change takes effect by restarting it ([ADR-0012](0012-worker-stop-and-config-by-restart.md)). Nothing in it is a credential, and nothing is written to it by the worker.

herdr-kata's `config.toml` (configuration.md) maps one or more Kata projects to a Herdr session, checkouts, profiles and limits. Its shape carries over; its route and Herdr parts do not.

## Shape

```toml
executor = "laptop"          # required, unique and stable; at most 256 bytes, no whitespace
actor    = "dangerboi"       # optional; default: $KATA_AUTHOR, $USER, git user.name, as `recur --as`

[limits]                     # all optional
max_active_runs = 4          # worker-wide cap; unset = Projects' capacities only
default_timeout = "2h"
max_timeout     = "8h"
max_run_prompts = 12         # 1 to 100

[[projects]]
name       = "tern-kata"     # required; resolved to a Project uid at first start and pinned
capacity   = 1               # default 1
actor      = "..."           # optional override
checkouts  = ["tern-kata"]   # optional allowlist of checkout keys; default all
profiles   = ["default"]     # optional allowlist of profile names; default all

[checkouts]                  # checkout_key -> absolute directory
tern-kata = "/home/me/Code/tern-plugins/tern-kata"

[profiles.default]           # at least `default`
command  = "omp"             # default "omp"
model    = "anthropic/claude-sonnet-5-5"
thinking = "medium"
args     = []                # extra arguments

[secrets]                    # name -> { env = "VAR" } or { file = "/abs/or/~/path" }
github-review = { file = "~/.config/tern-kata/github-review" }
```

## Decisions

- **Strict decode: adopt.** Unknown keys are an error. Checkout paths and `command` paths must be absolute or on `PATH`; limits positive; `max_timeout >= default_timeout`; every allowlisted name must exist; `profiles.default` must exist.
- **Trust: adopt, narrowed.** A listed Project is trusted, including its job and workflow authors (spec). herdr-kata's per-Project `checkouts` and `profiles` allowlists are kept because they stop a job in one Project from naming another Project's checkout by accident and feed the Job card's pickers; they are not a sandbox. Its `commands = false` switch is rejected: the spec has no command allowlists, and a Project that must not run commands should not be listed.
- **Secrets: adopt.** One `env` or `file` source per name; literal and `value` sources are refused; file sources must be regular, owned by the user, mode 0600, not symlinks; names match `[A-Za-z0-9._-]{1,256}`; a secret may not alias a kata credential source. `worker doctor` checks each source and prints its name, never its value (ADR-0009 owns resolution).
- **Route and credentials: reject herdr-kata's keys** (`[kata] bin`, `env`, per-Project `daemon`, `inbox`, `credential_env`, `credential_file`, `session`). The worker finds its daemon with kata's `client.Discover` as the Pane does (ADR-0001) and uses the credential that discovery selects. It runs no `kata` binary itself. `inbox` is only for native children (ADR-0002).
- **Pinned route: adapt.** At a Project's first start the worker saves, in `projects/<uid>/pin.json`, the Project uid, the daemon instance uid, the authenticated actor and the executor label. A later start whose Project name resolves to another uid, or whose daemon instance or actor differs, refuses admission and report delivery for that Project (a renamed Project keeps its pin by uid). A prepared run keeps the target it was frozen with. `worker doctor` names each differing field; there is no repin command, as in herdr-kata, and an intentional move uses a new state folder and executor.
- **First-start discovery that writes the config: reject.** herdr-kata writes a config enrolling every accessible Project that has a job or workflow. Here that would put Projects on the trusted list without the user choosing, which the spec's trust statement forbids. `worker doctor` lists the accessible Projects with adapter jobs and the stanza to add, and writes nothing.
- **Hot reload: reject; restart applies.** As herdr-kata. The worker reads the file once; the plugin host does not restart it on change: the user stops it with `tern-kata worker stop` and the host half starts a fresh one ([ADR-0012](0012-worker-stop-and-config-by-restart.md)). The Pane reads the same file for the Job card's profile and checkout pickers and falls back to free text when it is missing (spec).
- **Limits: adopt** `default_timeout` 2 h, `max_timeout` 8 h and `max_run_prompts` 12, each frozen into a run when it is prepared. herdr-kata's global `max_active_runs` default of 2 is dropped in favour of the spec's per-Project `capacity` default of 1 plus an optional worker-wide cap (ADR-0007). A profile's own `max_run_prompts` is not adopted; the cap is global.
- **Profiles: adapt.** herdr-kata's profile is a Herdr agent `kind` and `args`. A profile here is the omp command line: `command`, `model`, `thinking`, `args`, keeping room for other agents (spec). Job selection is `options.profile`, default `default`.
- **Board section: reject.** There is no pinned view.

## Consequences

- The state folder is `$XDG_STATE_HOME/tern-kata`, else `~/.local/state/tern-kata`, beside the config folder `os.UserConfigDir()/tern-kata` that `lanes.toml` already uses (`internal/data/lanes.go`). Both are overridable in tests.
- A user with Projects on two daemons runs one worker per daemon environment, as the Pane does today; federation stays out of scope.
