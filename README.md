# tern-kata

Browse and edit kata Issues in a native [Tern](https://docs.stencil.so/tern/) Pane.

## How it works

The plugin has two halves:

- **`tern-kata`**: a Go program that runs in a Pane. It talks to kata's daemon and draws the Issue list and detail with Tern's own widgets, using the Tern Surface Protocol. Run outside Tern, it falls back to `kata tui`.
- **Window plugin** (`window.luau`): adds **Open kata** to the palette and binds `F9`. Both open `tern-kata` in a split on the right, in the focused Pane's directory. It also opens kata links: `kata://<project>/<id>` or `https://kata.invalid/<project>/<id>` focuses the running kata Pane (or opens one) and selects that Issue.

In a directory bound with `kata init`, `tern-kata` opens that Project's Issues. Anywhere else it opens Inbox with a Project sidebar. Press `?` for the keybindings.

## Install

Requires Go 1.27+, kata v0.18.0+ and a Tern that supports the Surface Protocol.

1. On the **session host** (the machine whose shells run in your Panes), install kata, then:

   ```sh
   CGO_ENABLED=0 go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest
   ```

   `tern-kata` must be on your shell's `PATH` (Go puts it in `~/go/bin`, or `%USERPROFILE%\go\bin` on Windows).

2. On the **machine showing the Tern window**, install the window plugin:

   ```sh
   tern plugin install github.com/Dangerboi/tern-kata
   ```

The session host can run Linux, macOS or Windows. If it is the same machine as the window, you can do both steps on that one machine.

## Development

```sh
CGO_ENABLED=0 go build -o tern-kata ./cmd/tern-kata
CGO_ENABLED=0 go test ./...
./smoke/run.sh      # end-to-end against a headless Tern and a throwaway kata daemon
tern plugin link .  # use this checkout as the window plugin
```
