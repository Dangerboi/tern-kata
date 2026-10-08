#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
pid=
export CGO_ENABLED=0 KATA_HOME="$tmp/kata" TERN_CONFIG_DIR="$tmp/config"
export TERN_DAEMON_SOCKET="$tmp/daemon.sock" STENCIL_LOG_DIR="$tmp/logs"
export STENCIL_FIXTURE_ROOT="$tmp/crates/tern"
export STENCIL_LOG=warn,stencil_plugins=debug,stencil_tern=debug
unset KATA_SERVER KATA_AUTH_TOKEN KATA_AUTHOR TMUX STY ZELLIJ TERN_TSP
cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "$pid" ]]; then
    if [[ $status -ne 0 ]]; then
      ctl tree >"$tmp/tree.json" 2>/dev/null || true
      ctl state >"$tmp/state.json" 2>/dev/null || true
    fi
    ctl quit >/dev/null 2>&1 || true
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  if [[ -f "$TERN_CONFIG_DIR/plugins/tern-kata.path" ]]; then
    tern plugin unlink tern-kata --json || status=1
    [[ ! -f "$TERN_CONFIG_DIR/plugins/tern-kata.path" ]] || status=1
  fi
  kata daemon stop >/dev/null 2>&1 || true
  if [[ $status -eq 0 ]]; then rm -rf "$tmp"; else printf 'Plugin smoke artifacts: %s\n' "$tmp" >&2; fi
  exit "$status"
}
trap cleanup EXIT
ctl() { tern ctl --control "$tmp/control.sock" "$@"; }
mkdir -p "$tmp/workspace" "$tmp/bin" "$tmp/pane-bin" "$tmp/app" "$tmp/crates/tern/goldens" "$tmp/crates/plugins/fixtures"
if [[ $# -gt 0 ]]; then cp "$1" "$tmp/app/tern-kata"; else (cd "$repo" && go build -o "$tmp/app/tern-kata" .); fi
printf '#!/bin/sh\nexec env KATA_HOME=%q %q "$@"\n' "$KATA_HOME" "$tmp/app/tern-kata" >"$tmp/pane-bin/tern-kata"
chmod +x "$tmp/pane-bin/tern-kata"
ln -s "$repo" "$tmp/crates/plugins/fixtures/tern-kata"
ln -s "$(command -v kata)" "$tmp/bin/kata"
ln -s "$(command -v tern)" "$tmp/bin/tern"

if command -v zsh >/dev/null 2>&1; then
  export SHELL="$(command -v zsh)"
fi
# Window process PATH lacks tern-kata; Pane shells resolve it via their shell rc.
export ZDOTDIR="$tmp" BASH_ENV="$tmp/.bashrc" ENV="$tmp/.bashrc"
printf 'export PATH=%q:%q:"$PATH"\n' "$tmp/pane-bin" "$tmp/bin" >"$tmp/.zshenv"
printf 'export PATH=%q:%q:"$PATH"\n' "$tmp/pane-bin" "$tmp/bin" >"$tmp/.bashrc"
export PATH="$tmp/bin:/usr/bin:/bin"
kata daemon start
kata --workspace "$tmp/workspace" init --project smoke --json
kata --workspace "$tmp/workspace" create 'Plugin first Issue' --body 'Plugin first body' --json >"$tmp/first.json"
kata --workspace "$tmp/workspace" create 'Plugin second Issue' --body 'Plugin second body' --json >"$tmp/second.json"
first=$(jq -r '.issue.short_id' "$tmp/first.json")
second=$(jq -r '.issue.short_id' "$tmp/second.json")
(cd "$repo" && tern plugin link . --json)
! command -v tern-kata >/dev/null 2>&1 || { echo "tern-kata must not be on window PATH" >&2; exit 1; }
tern serve --control "$tmp/control.sock" --out "$tmp/shots" --dir "$tmp/workspace" >"$tmp/tern.log" 2>&1 &
pid=$!
for ((attempt=0; attempt<200; attempt++)); do
  [[ ! -S "$tmp/control.sock" ]] || break
  kill -0 "$pid"
  sleep 0.1
done
[[ -S "$tmp/control.sock" ]]
ctl ready
ctl plugins fixtures
printf 'palette "Open kata"\nkey Enter\n' >"$tmp/palette.ctl"
ctl --file "$tmp/palette.ctl"
ctl plugins expect smoke
ctl state >"$tmp/palette-state.json"
jq -e --arg cwd "$tmp/workspace" '.tabs[0].panes == 2 and .focused.cwd == $cwd' "$tmp/palette-state.json" >/dev/null
palette_pane=$(jq -r '.focused.id' "$tmp/palette-state.json")
ctl key q
ctl ready
ctl focus left
ctl key F9
ctl plugins expect smoke
ctl state >"$tmp/key-state.json"
jq -e --arg cwd "$tmp/workspace" --argjson previous "$palette_pane" '.tabs[0].panes == 2 and .focused.cwd == $cwd and .focused.id != $previous' "$tmp/key-state.json" >/dev/null
ctl key q
ctl ready
ctl focus left
# A real OSC-8 terminal link exercises route.link, not a plugin-origin cx:open.
click_issue_link() {
  command="printf '\\033[2J\\033[H\\033]8;;kata://smoke/$1\\007Plugin link\\033]8;;\\007\\n'"
  printf 'run %s\n' "$(jq -rRn --arg command "$command" '$command | @json')" >"$tmp/link.ctl"
  ctl --file "$tmp/link.ctl"
  ctl expect Plugin
  ctl tree '.tv-grid' >"$tmp/link-tree.json"
  read -r x y < <(jq -r '.nodes[0].rect | "\(.[0]+4) \(.[1]+8)"' "$tmp/link-tree.json")
  ctl click "$x" "$y" cmd
}
click_issue_link "$first"
ctl plugins expect first
ctl state >"$tmp/link-open.json"
kata_pane=$(jq -r '.focused.id' "$tmp/link-open.json")
jq -e '.tabs[0].panes == 2' "$tmp/link-open.json" >/dev/null
ctl focus left
click_issue_link "$second"
ctl plugins expect second
ctl state >"$tmp/link-focus.json"
jq -e --argjson id "$kata_pane" '.tabs[0].panes == 2 and .focused.id == $id' "$tmp/link-focus.json" >/dev/null
ctl key q
ctl ready
rm "$tmp/pane-bin/tern-kata"
ctl plugins run plugin.tern-kata.open
ctl expect '"go install github.com/Dangerboi/tern-kata@latest"'
jq -Rse 'contains("plugin window halves loaded count=1") and (contains("plugin bind left out") | not) and (contains("plugin hook exceeded its budget") | not)' "$tmp/tern.log" >/dev/null
printf 'Plugin smoke passed: consecutive palette/key splits without reload, real link open/focus, missing-binary in-Pane hint\n'
