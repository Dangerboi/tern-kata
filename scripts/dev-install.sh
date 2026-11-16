#!/usr/bin/env bash
# Installs this checkout as the dev build and checks that every machine runs it: run it before asking the user to try a change.
#   This machine: `tern plugin link` on the checkout (the link reloads on file changes), and the binary rebuilt over the
#                 installed tern-kata when one is on the PATH (a running worker is then stopped when idle, as `tern-kata update` does).
#   Windows window machine (optional): the plugin folder is copied over ssh and installed with `tern plugin install --force`, and
#                 the Windows binary replaced when tern-kata is installed there (the running exe renamed aside, as install.ps1 does).
# The ssh target (user@host or an ssh config alias) comes from TERN_KATA_WINDOWS_SSH or, when that is unset, from
# .dev-install.local in the repo root (gitignored): one line, TERN_KATA_WINDOWS_SSH=user@host. Without a target the Windows half is skipped.
# It then prints, per machine, the installed plugin version and a digest of the plugin files against this checkout's
# (commit and version), and exits 1 on any mismatch.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
# The user's Tern links the plugin from here and runs this build, so only the primary checkout installs: a side checkout
# (a linked worktree or an ovnfs checkout) would leave the link on a folder that is later removed.
side=$(command -v ovnfs >/dev/null && ovnfs ls 2>/dev/null | awk 'NR > 1 { print $NF }' | grep -xF "$repo" || true)
if [[ $(git rev-parse --path-format=absolute --git-dir) != "$(git rev-parse --path-format=absolute --git-common-dir)" || -n $side ]]; then
  printf 'dev-install.sh: %s is a side checkout; merge the change and run it from the primary checkout\n' "$repo" >&2
  exit 1
fi
export CGO_ENABLED=0
# One line parsed out, not sourced: the file is the user's, and only this one setting is wanted from it.
target=${TERN_KATA_WINDOWS_SSH:-}
if [[ -z $target && -f .dev-install.local ]]; then
  target=$(sed -n 's/^TERN_KATA_WINDOWS_SSH=//p' .dev-install.local | tail -n 1 | tr -d "\"'")
fi

# shellcheck source=scripts/plugin-info.sh
. scripts/plugin-info.sh
version=$(plugin_version plugin.toml)
commit=$(git rev-parse --short HEAD)
[[ -z $(git status --porcelain --untracked-files=no) ]] || commit+="+dirty"
# Stamped like the dev builds before this script (v0.13.0-1-gec45caf-dev), so --version names the commit.
ldflags="-X main.version=$(git describe --tags --dirty --always)-dev"
status=0

tmp=$(mktemp -d)
new_bin=
trap 'rm -rf "$tmp" ${new_bin:+"$new_bin"}' EXIT

# "name sha256" lines for each plugin file, the form both machines are compared in.
local_digest=$(for f in $plugin_files; do printf '%s %s\n' "$f" "$(sha256sum "$f" | cut -d ' ' -f 1)"; done | sort)

report() { # report <machine> <plugin version> <plugin digest> [binary digest of this build, binary digest installed]
  local machine=$1 installed=$2 digest=$3
  if [[ $installed == "$version" && $digest == "$local_digest" ]]; then
    printf '%s: plugin %s matches checkout %s (%s)\n' "$machine" "$installed" "$version" "$commit"
  else
    printf '%s: MISMATCH plugin %s, checkout %s (%s)\n' "$machine" "${installed:-none}" "$version" "$commit" >&2
    diff <(printf '%s\n' "$digest") <(printf '%s\n' "$local_digest") >&2 || true
    status=1
  fi
  if [[ $# -ge 5 ]]; then
    if [[ $4 == "$5" ]]; then
      printf '%s: binary matches this build\n' "$machine"
    else
      printf '%s: MISMATCH binary %s, this build %s\n' "$machine" "$5" "$4" >&2
      status=1
    fi
  fi
}

# This machine.
tern plugin link .
tern plugin reload tern-kata
local_installed=$(tern plugin list --json | jq -r '.plugins[] | select(.id == "tern-kata") | .version')
linked=$(cat "$(tern plugin dir)/tern-kata.path" 2>/dev/null || true)
if [[ $linked != "$repo" ]]; then
  printf 'this machine: MISMATCH tern-kata is not linked to %s\n' "$repo" >&2
  status=1
fi
args=("this machine" "$local_installed" "$local_digest")
if bin=$(command -v tern-kata); then
  # Built beside the installed file, so the final mv is a same-filesystem rename; a symlinked install keeps its link.
  bin=$(readlink -f "$bin")
  new_bin=$bin.new
  go build -trimpath -ldflags "$ldflags" -o "$new_bin" ./cmd/tern-kata
  want=$(sha256sum "$new_bin" | cut -d ' ' -f 1)
  mv "$new_bin" "$bin"
  new_bin=
  args+=("$want" "$(sha256sum "$bin" | cut -d ' ' -f 1)")
  # As `tern-kata update` does: the worker keeps running the old build until it stops, and the plugin starts a fresh one.
  if stopped=$("$bin" worker stop 2>&1); then printf 'this machine: %s\n' "$stopped"; fi
fi
report "${args[@]}"

# The Windows window machine. The remote shell is PowerShell: commands go in single quotes so bash leaves $ alone, run
# from the home folder (a cwd holding the plugin folder fails the install with os error 32), and use `;`, never `cd … &&`.
if [[ -z $target ]]; then
  printf 'Windows: skipped (set TERN_KATA_WINDOWS_SSH or .dev-install.local)\n'
else
  remote() { ssh -o BatchMode=yes "$target" "$1"; }
  stage=.tern-kata-dev-plugin
  remote "Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue; New-Item -ItemType Directory $stage | Out-Null"
  # shellcheck disable=SC2086
  scp -q -o BatchMode=yes $plugin_files "$target:$stage/"
  remote "tern plugin install $stage --force"
  remote "Remove-Item -Recurse -Force $stage"
  if remote 'if (Get-Command tern-kata -ErrorAction SilentlyContinue) { "yes" }' | grep -q yes; then
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$tmp/tern-kata.exe" ./cmd/tern-kata
    scp -q -o BatchMode=yes "$tmp/tern-kata.exe" "$target:.tern-kata-dev.exe"
    # As install.ps1 does: a running exe can only be renamed, and an .old still held by a process is renamed aside.
    remote '$d = (Get-Command tern-kata).Source; Get-ChildItem -LiteralPath (Split-Path $d) -Filter "tern-kata.exe.old-*" -ErrorAction SilentlyContinue | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }; Remove-Item -LiteralPath "$d.old" -Force -ErrorAction SilentlyContinue; if (Test-Path -LiteralPath "$d.old") { Move-Item -LiteralPath "$d.old" "$d.old-$([DateTime]::UtcNow.Ticks)" -Force }; Move-Item -LiteralPath $d "$d.old" -Force; try { Move-Item -LiteralPath .tern-kata-dev.exe $d -ErrorAction Stop } catch { Move-Item -LiteralPath "$d.old" $d -Force; throw }'
    want=$(sha256sum "$tmp/tern-kata.exe" | cut -d ' ' -f 1)
    have=$(remote '(Get-FileHash (Get-Command tern-kata).Source -Algorithm SHA256).Hash.ToLower()' | tr -d '\r')
    bin_args=("$want" "$have")
    # As on this machine: the running worker keeps the old build until it stops, and the plugin starts a fresh one.
    if stopped=$(remote 'tern-kata worker stop 2>&1' | tr -d '\r'); then printf 'Windows: %s\n' "$stopped"; fi
  else
    bin_args=()
  fi
  win_version=$(remote 'tern plugin list --json' | jq -r '.plugins[] | select(.id == "tern-kata") | .version')
  win_digest=$(remote '$p = tern plugin dir; Get-ChildItem "$p\tern-kata" -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower())" }' | tr -d '\r' | awk -v names=" $plugin_files " 'index(names, " " $1 " ") { print }' | sort)
  report "Windows" "$win_version" "$win_digest" ${bin_args[@]+"${bin_args[@]}"}
fi

exit "$status"
