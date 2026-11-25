# Development

```sh
CGO_ENABLED=0 go build -o tern-kata ./cmd/tern-kata
CGO_ENABLED=0 go test ./...
bun test internal/data/ompext # the omp extension tern-kata loads into its agents (TypeScript, embedded in the binary)
./smoke/run.sh               # end-to-end against headless Tern servers and throwaway kata daemons
scripts/dev-install.sh       # install this checkout as the dev build here and on the Windows window machine, then compare versions
scripts/check.sh             # everything: static (gofmt, vet, golangci-lint, every Luau test, builds), test or smoke alone with that argument
tern plugin link .           # use this checkout as the window plugin; changing a file in it reloads it (from Tern 0.9.16, `git status` and other changes under .git/ don't)
tern plugin reload tern-kata # exits 1 if it fails to load; window failures show only as a toast
```

`scripts/check.sh` ends every mode with one line, `CHECKS ok` or `CHECKS FAIL: <names>` (the failed static checks, named by label, such as `fmt`, `lint-windows` or `luau-tests`, or the failed group for test, smoke and all); the exit status is unchanged.

`scripts/dev-install.sh` links this checkout as the window plugin (`tern plugin link`, reloaded) and rebuilds the installed `tern-kata` binary over the one on the `PATH` (built beside it and moved into place; a running worker is stopped when idle, as `tern-kata update` does, so the plugin starts one from the new build), stamped `<git describe>-dev` (`tern-kata --version` names the commit; `tern-kata update` offers the release over it). When `TERN_KATA_WINDOWS_SSH` (an ssh target such as `user@host` or an ssh config alias) is set in the environment or in `.dev-install.local` at the repo root (gitignored, one line: `TERN_KATA_WINDOWS_SSH=user@host`), it also copies the plugin files to that Windows machine, runs `tern plugin install --force` there from the home folder, and replaces its `tern-kata.exe` when one is installed (renamed aside as `install.ps1` does, and its running worker stopped when idle). It prints per machine whether the installed plugin's version and file digests match this checkout and exits 1 on a mismatch; without a target the Windows half is skipped. It refuses a linked worktree or an ovnfs checkout: the link would outlive that folder. Run it from the primary checkout before you ask the user to try a change: the Windows machine runs its own plugin copy.

Every push to `main` and every pull request runs `.github/workflows/ci.yml` as two jobs side by side: `static` (`scripts/check.sh static`: gofmt, `go vet` and golangci-lint (`.golangci.yml`) for Linux and for Windows, every Luau test in `test/*_test.luau` (`luau`, release 0.745 downloaded by the job), the build and the release cross-builds, all at once, with golangci-lint v2.15.0 installed by `go install`) and `test` (`go test ./...`, then `bun test internal/data/ompext` on Bun 1.4.4). Run `scripts/check.sh static` locally for the same checks (it needs `luau` on the PATH); `scripts/check.sh test` prints the output of failing tests (it needs `bun` on the PATH), and without `CHECK_OUT` the script deletes its temporary output folder on exit. Run `golangci-lint run ./...` locally with a golangci-lint built by Go 1.27 or newer; an older build cannot load this toolchain's standard library. The smoke suite needs Tern and a kata daemon, so it runs only locally; it needs Tern 0.9.12 or newer (0.9.21 for the worker widget the host half draws, which `plugin.sh` checks only from that version), whose busy-close question `smoke/steps/35-close.sh` finds as Tern's own dialog in the window. `smoke/plugin.sh` also needs zsh: its Pane shells redraw their prompt while idle, because headless Tern runs the window plugin's timers only when it draws a frame ([stencil-hq/tern-sdk#298](https://github.com/stencil-hq/tern-sdk/issues/298)).

`./smoke/run.sh` with no arguments runs the steps as parallel shards, each on its own headless Tern and kata daemon: five runs of consecutive UI steps (cut at 22, 32, 45 and 58), one run each for the steps that wait on kata's daemon clock (51, 60, 62, 81, 82), and `plugin.sh`, `supervision.sh` and `release-check.sh`. A step that needs state left by an earlier step must stay in that step's shard. `./smoke/run.sh STEP…` runs only the given steps, in order, on one server (seconds for one step); `plugin.sh`, `supervision.sh` and `release-check.sh` run in an argument run only when named among the arguments (`./smoke/run.sh smoke/steps/72-x.sh smoke/supervision.sh`), after the steps and handed the binary built once. A failed run ends with a line naming the failing step or script and the artifacts folder (`Smoke failed at 72-quit.sh; artifacts: /tmp/tmp.X`); the full run's last line names every failed shard's step or script. A passing full run ends with its time and its three slowest jobs (`Smoke passed in 189 s; slowest: 62-job-card.sh 181 s, …`): the slowest job sets the suite's time, and step 62 (about 180 s, on kata's 30 s delivery sweep) is the floor, so a job that grows past it makes every run longer. The full run builds the release `dist/` once (`scripts/release.sh`) and hands it to steps 81 and 82 and `supervision.sh` in `SMOKE_DIST`; run on their own they build their own. Tern's own `ready` and `expect` waits stop after a fixed 20 s, which a loaded machine can exceed, so `smoke/ctl.sh` asks them again until `SMOKE_WAIT_LIMIT` seconds (default 120) have passed; the tree, hint-mode and quit waits in `run.sh` use the same deadline.

To let time pass for the window plugin's timers in `plugin.sh`, sleep: headless Tern runs them only on frames, and its shells' idle zsh prompts redraw every 0.1 s, so frames keep coming. A `run` line in a ctl file waits a fixed 1.2 s after it types, so a loop of them is a slow way to wait. `pick_option <field id> <nth>` in `smoke/run.sh` picks a popup card's select option with clicks, each acknowledged by the Pane (the footer offers "pick choice" once the menu is open), so a step reads the Pane's state before its next click.

Smoke waits go through `wait_for` (`smoke/ctl.sh`), which polls until its command succeeds and gives up after `SMOKE_WAIT_LIMIT` seconds (default 120), naming the command. A short fixed wait or `sleep` is allowed only where the step proves a bound (something must happen, or not happen, within that time); a comment beside it says so. A sleep with no state to poll says that no wait is possible and that it is not a bound. `plugin.sh` and `release-check.sh` call `trace_failures` (`smoke/ctl.sh`), so a command that ends the script unhandled prints its file, line and command.

`run.sh`, `plugin.sh` and `supervision.sh` export `SMOKE_OWNER=$$` before they start a daemon, and every kata or Docbank daemon they start inherits it. Each script's cleanup runs `smoke/sweep-daemons.sh <its pid>`, which also stops daemons a failed step left running. A run killed before its cleanup (SIGKILL, the OOM killer) leaves its daemons running, so `./smoke/run.sh` (except inside a shard) begins with `smoke/sweep-daemons.sh` and no owner: that stops every smoke daemon whose owner script is gone. Daemons of runs still going are left alone. A step that starts a daemon needs no stop of its own.

## Releases

Pushing a `vX.Y.Z` tag runs `.github/workflows/release.yml` (vet, test, then publish), with the tag's `CHANGELOG.md` section as the release notes. Check it locally before tagging:

```sh
scripts/release-notes.sh vX.Y.Z   # prints the notes; fails unless CHANGELOG.md has a [X.Y.Z] section
scripts/release.sh vX.Y.Z         # writes dist/ (gitignored); fails unless plugin.toml's version is X.Y.Z and the notes exist; publishes nothing
```

It builds `tern-kata_<os>_<arch>.tar.gz` (`.zip` for `windows`) for `linux`, `darwin` and `windows` on `amd64` and `arm64`, each also holding `LICENSE` and a `plugin/` folder (`plugin.toml` and the root `.luau` files: `plugin_files` in `scripts/plugin-info.sh` is the list) that `install.sh` and `install.ps1` hand to `tern plugin install`, plus `checksums.txt` in `sha256sum` format (archives only) and `version.txt`, the plugin's version as `plugin.toml` spells it, then a newline. The script refuses to build when a `require("./name")` in those files names a file `plugin/` lacks: add a new plugin file to `plugin_files` in `scripts/plugin-info.sh`. `smoke/steps/81-install-from-release.sh` builds `dist/` at the current version, serves it locally and runs the install scripts' Linux path against it, plugin-only mode included; `install.ps1` is checked by hand on Windows against a locally served `dist/` (`TERN_KATA_BASE_URL`).

A release build sets the version with `-ldflags "-X main.version=vX.Y.Z"`. Without it, `tern-kata --version` reports the Go module version (`go install …@vX.Y.Z`), or `dev` for an untagged build.

## Type-checking the plugin

Every root `.luau` file and every file in `test/` (`test/harness.luau`, the shared harness every test requires, included) is `--!strict`. To type-check them against the installed Tern's API (needs [luau-lsp](https://github.com/JohnnyMorganz/luau-lsp)):

```sh
tern plugin types .  # writes tern.d.luau (gitignored; regenerate per Tern version)
luau-lsp analyze --platform=standard --definitions=@tern=tern.d.luau --ignore=tern.d.luau ./*.luau test/*.luau
```

`placement.luau` holds the rule that places an agent's tab as plain data, with no Tern call. Its tests run outside Tern with the [Luau](https://luau.org) command line (`brew install luau`; CI installs release 0.745):

```sh
scripts/check.sh static   # runs every test/*_test.luau
luau test/placement_test.luau   # or one alone
```

## Design records

The [ADRs](../adr) record the design decisions behind the plugin and the worker.
