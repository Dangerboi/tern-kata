# tern-kata

Browse and edit [kata](https://katatracker.com/) Issues in a native [Tern](https://docs.stencil.so/tern/) Pane.

kata ([source](https://github.com/kenn-io/kata)) is an issue tracker built for coding agents and the people steering them. Agents create, claim, relate and close Issues from its CLI, and people follow the same work in its terminal UI, `kata tui`. By default Issues live in a local SQLite database rather than in your repo, and no hosted service is needed. tern-kata shows that work in a Tern Pane drawn with Tern's own widgets instead of terminal text.

The same example Project in tern-kata (top) and in `kata tui` (bottom), both running in Tern:

![tern-kata showing the acme-shop Project with "Add Apple Pay to checkout" selected](docs/images/tern-kata.png)

![kata tui showing the same Project and Issue](docs/images/kata-tui.png)

In tern-kata, Issue bodies render as Markdown, priorities and labels show as chips, and rows, tabs and links respond to mouse clicks.

## How it works

The plugin has two halves:

- **`tern-kata`**: a Go program that runs in a Pane. It talks to kata's daemon and draws the Issue list and detail with Tern's own widgets, using the Tern Surface Protocol. Run outside Tern, it falls back to `kata tui`.
- **Window plugin** (`window.luau`): adds **Open kata** to the palette and binds `F9`. Both open `tern-kata` in a split on the right, in the focused Pane's directory. It also opens kata links: `kata://<project>/<id>` or `https://kata.invalid/<project>/<id>` focuses the kata Pane in the current tab (or opens one there) and selects that Issue. A kata Pane in another tab is left alone.

In a directory bound to a kata Project, `tern-kata` opens that Project's Issues. A directory is bound when it, or a parent up to the enclosing git root, contains a `.kata.toml`, which `kata init` creates. Anywhere else it opens Inbox beside the Menu sidebar, where you can switch to another view (Today, All Open, Logbook and others), browse your kata Projects or create a new one. In a bound directory the same Menu opens as a popup with `esc`, and `P` opens it straight at the list of Projects. Press `?` for the keybindings.

## Install

Requires kata v0.19.0+ and a Tern that supports the Surface Protocol.

1. On the **session host** (the machine whose shells run in your Panes), install kata, then tern-kata. On Linux or macOS:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.sh | sh
   ```

   On Windows, in PowerShell:

   ```powershell
   irm https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.ps1 | iex
   ```

   They install the latest release to `~/.local/bin` (`%LOCALAPPDATA%\Programs\tern-kata` on Windows, added to your user `PATH`), verify its checksum, and also install the window plugin when `tern` is on `PATH`. Once the plugin is installed, the **Install tern-kata** palette command runs the same one-liner in a new Pane. `tern-kata --help` lists the flags.

   Alternatively, with Go 1.27+:

   ```sh
   CGO_ENABLED=0 go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest
   ```

   Go puts the binary in `~/go/bin` (`%USERPROFILE%\go\bin` on Windows); it must be on your shell's `PATH`.

2. On the **machine showing the Tern window**, install the window plugin:

   ```sh
   tern plugin install github.com/Dangerboi/tern-kata
   ```

The window machine needs only the window plugin; kata and tern-kata run on the session host. For example, with a Tern window on Windows 11 and Panes on an Ubuntu host, install the window plugin on Windows and kata and tern-kata on Ubuntu. If the session host and the window are the same machine, do both steps on it.

## Install with an agent

In a Pane on the session host, give your coding agent this prompt:

```text
Install tern-kata by following the "Install with an agent" section of https://raw.githubusercontent.com/Dangerboi/tern-kata/main/README.md
```

Agents: follow these steps only when the user has asked you to install tern-kata.

1. Check that `kata` is on `PATH`. If it isn't, stop and tell the user that tern-kata requires kata, which they install themselves: https://katatracker.com/.
2. Run the install one-liner from [Install](#install) step 1 for this machine's OS.
3. Run `tern-kata --version`. If it isn't found, pass on the `PATH` instructions the install script printed.
4. If the script reported that `tern` is not on `PATH`, tell the user to run `tern plugin install github.com/Dangerboi/tern-kata` on the machine that shows the Tern window. Without it, the Open kata command, `F9` and kata links don't work.
5. Ask the user whether to add the rule from [Clickable ticket links from agents](#clickable-ticket-links-from-agents) to your global rules file, the one you load in every repo. The links only work when your chat is shown in Tern with the window plugin installed. Copy the rule block exactly, and skip it if the file already contains it.

## Clickable ticket links from agents

Tern draws `kata://` links in chat as plain text, so agents should write ticket references as `https://kata.invalid/...` links, which the plugin intercepts. To make your agent do that in every kata-tracked repo, add this to its global rules file (`~/.omp/agent/RULES.md` for OMP):

```markdown
In a repo with a `.kata.toml`, write kata ticket references in chat replies as `[<id>](https://kata.invalid/<project>/<id>)`, with `<project>` the name in `.kata.toml`. The tern-kata window plugin opens them in tern-kata; Tern draws `kata://` links in chat as plain text. Everywhere else (files, commit messages, kata Issue text) write the bare id; those links only work inside Tern.
```

## Compared with `kata tui`

tern-kata takes kata tui's movement keys: `j`/`k` and the arrows, `ctrl+n`/`ctrl+p` to move a row, `pgup`/`pgdn` and `alt+v`/`ctrl+v` to page, `g`/`G`, and `I` to open the Inbox (from the list, the Issue detail and the Menu sidebar; tern-kata opens it, where kata tui toggles it). `?` lists every key. The Emacs keys and `I` are plain text in search and in Popup cards.

It lacks these kata tui keys:

- `u`: undo the last change.
- `t`: timed assignment.
- `R`: typed reply to a comment.
- `[` and `]`: previous and next comment link.

## Known issues

- **Windows: each new kata Pane takes a moment to open.** Tern starts a Pane's command through PowerShell and can't yet pass `-NoProfile`, so every new kata Pane loads your PowerShell profile before tern-kata starts; a large profile makes the wait longer. Once open, the Pane runs at full speed. A fix has been requested from the Tern developers.

## Development

```sh
CGO_ENABLED=0 go build -o tern-kata ./cmd/tern-kata
CGO_ENABLED=0 go test ./...
./smoke/run.sh      # end-to-end against a headless Tern and a throwaway kata daemon
tern plugin link .  # use this checkout as the window plugin
tern plugin reload  # exits 1 if a plugin fails to load; window failures show only as a toast
```

Releases: pushing a `vX.Y.Z` tag runs `.github/workflows/release.yml` (vet, test, then publish). Check it locally before tagging:

```sh
scripts/release.sh vX.Y.Z   # writes dist/ (gitignored); fails unless plugin.toml's version is X.Y.Z; publishes nothing
```

It builds `tern-kata_<os>_<arch>.tar.gz` (`.zip` for `windows`) for `linux`, `darwin` and `windows` on `amd64` and `arm64`, plus `checksums.txt` in `sha256sum` format.

A release build sets the version with `-ldflags "-X main.version=vX.Y.Z"`. Without it, `tern-kata --version` reports the Go module version (`go install …@vX.Y.Z`), or `dev` for an untagged build.

`window.luau` is `--!strict`. To type-check it against the installed Tern's API (needs [luau-lsp](https://github.com/JohnnyMorganz/luau-lsp)):

```sh
tern plugin types .  # writes tern.d.luau (gitignored; regenerate per Tern version)
luau-lsp analyze --platform=standard --definitions=@tern=tern.d.luau --ignore=tern.d.luau window.luau
```
