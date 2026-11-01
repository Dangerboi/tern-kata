# tern-kata updates from its own releases: the binary swaps itself, the plugin comes from the release, and only the window half checks

Before this, updating meant running the install one-liner again on every machine. The binary came from the latest release. The plugin came from `tern plugin install github.com/…`, which clones `main`, so between releases it could be newer than the binary it launches. On a split setup the window machine's copy was always a separate, manual step. `tern-kata update` and an Update palette command replace that with one path covering the session host and the window machine.

## Decisions

- **One version, one release.** The binary and the plugin keep sharing `plugin.toml`'s version (`scripts/release.sh` refuses a tag that differs). Every OS archive carries the plugin in a `plugin/` folder, and each release has a `version.txt`. Plugins are installed from a release archive with `tern plugin install <dir> --force`, never from a clone of `main`, so a plugin is never newer than its release.
- **The binary replaces itself.** `tern-kata update` reads `version.txt` and downloads that tag's archive and `checksums.txt`. It checks SHA-256 the way the install scripts do: that catches corruption, not a compromised release, which would serve the scripts too. Then it swaps the file. On Unix the new file is renamed over the old one. On Windows the running `.exe` is renamed to `.old`. A `.old` still held by a running process can't be deleted ("Access is denied") but can be renamed aside, both observed on Windows 11. Only release builds, whose version the linker stamped, are replaced; other builds get the command for how they were installed. It only moves forward, to the latest stable release.
- **Pre-flight is the new binary's call.** Before the swap, the old binary runs the downloaded one in pre-flight mode, which checks this host against the new binary's own requirements: the kata daemon's version against its minimum, and `worker.toml` against its loader. A refusal stops the swap unless `--force` is given. An unreachable daemon gives a warning, not a refusal. The call's form and exit statuses are a contract between releases: a new release may add to them but never change them.
- **Binary before plugin, session host before window machine.** The harmful mismatch is a plugin newer than the binary it launches, because inside Tern an older binary refuses arguments it doesn't know. So `update` swaps the binary before reinstalling the session host's plugin, and the palette command updates the window machine's plugin only after the session host's update succeeded.
- **Only installed, unlinked plugin copies.** An update reinstalls the plugin where a copy is installed (a `tern-kata` folder in `tern plugin dir`). It never touches a linked one (`tern-kata.path`) and never adds one where there was none.
- **The Worker moves to the new binary by its own stop.** After the swap, a supervised Worker gets an `idle` stop (ADR-0012), and Supervision starts the new binary. A Worker started by hand gets a note to restart it. If pre-flight found that `worker.toml` must change and `--force` swapped anyway, the Worker is left running its old code. The user is told to run `worker upgrade`, then `worker stop`. `update` never runs `worker upgrade` itself, because that command writes trust grants.
- **The window half is the only release checker.** It reads `version.txt` at most once a day per window machine (state in `tern.kv`), toasts once per new version and stays silent on failure. A palette toggle turns the check off, and `TERN_KATA_NO_UPDATE_CHECK` overrides the toggle. A plugin linked to a checkout (`tern plugin link`) never checks, because that means a development machine, where dev builds are installed on purpose. The binary never checks for releases on its own.
- **No version handshake.** The window half can't learn a binary's version: reading a Pane exposes no app version, and `Launch` sets no environment. A new launch argument would stop older binaries from starting. The start failure toast says the binary may be older than the plugin and names Update instead.
- **Rejected:**
  - Running the install scripts from the binary: that runs whatever script is on `main` and needs curl or PowerShell.
  - Moving all install logic into Go now: Windows `PATH` registration would have to move too.
  - Signatures or attestations: the scripts that make the first install check no more.
  - A release check in the Pane: a second checker in a second language, with a second opt-out.
  - Running `worker upgrade` automatically.

## Consequences

- The install scripts take the plugin from the release archive and gain a plugin-only mode for window machines. The README's `tern plugin install github.com/…` step becomes that mode. `tern plugin link .` stays the development path.
- A plugin-only install downloads its OS archive (~12 MB) for a few Luau files.
- A session host nobody updated keeps an older binary while the window plugin moves on. With several session hosts this is expected, and the start failure toast is the guard.
- `TERN_KATA_BASE_URL` points the scripts, `update` and the window half's check at a local `dist/`, which is how the smoke tests drive them.
- `install.ps1` fails today on a held `.old` ("Cannot create a file when that file already exists") and adopts the same rename-aside.
