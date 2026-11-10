#!/bin/sh
# Installs the latest tern-kata release for Linux or macOS: curl -fsSL https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.sh | sh
# TERN_KATA_BASE_URL replaces the download location (default: the latest GitHub release), TERN_KATA_INSTALL_DIR the install folder.
# TERN_KATA_PLUGIN_ONLY=1 installs only the Tern window plugin from the release, for the machine that shows the Tern window.
set -eu

die() {
	echo "install.sh: $*" >&2
	exit 1
}

fetch() { # $1 url, $2 output file
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		die "need curl or wget to download tern-kata"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		die "need sha256sum or shasum to verify the download"
	fi
}

# Installs the plugin folder $1 with tern; a linked copy, which --force leaves alone, gets a message. Returns 1 on any other failure.
install_plugin() {
	if out=$(tern plugin install "$1" --force 2>&1); then
		echo "$out"
	else
		case $out in
		*"is linked"*) echo "The tern-kata window plugin is linked, so it is left as it is: $out" ;;
		*)
			echo "$out" >&2
			return 1
			;;
		esac
	fi
}

# The whole script is one function called last, so a download cut short runs nothing.
main() {
	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "unsupported OS $(uname -s): this script supports Linux and macOS (on Windows use install.ps1)" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported CPU $(uname -m): tern-kata is released for amd64 and arm64" ;;
	esac

	base=${TERN_KATA_BASE_URL:-https://github.com/Dangerboi/tern-kata/releases/latest/download}
	dir=${TERN_KATA_INSTALL_DIR:-$HOME/.local/bin}
	asset=tern-kata_${os}_$arch.tar.gz

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	echo "Downloading $asset"
	fetch "$base/$asset" "$tmp/$asset" || die "could not download $base/$asset"
	fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "could not download $base/checksums.txt"

	want=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")
	[ -n "$want" ] || die "checksums.txt has no entry for $asset"
	got=$(sha256 "$tmp/$asset")
	[ "$got" = "$want" ] || die "checksum mismatch for $asset (expected $want, got $got); nothing was installed"

	# Releases before the window plugin was shipped have no plugin/ folder.
	if tar -tzf "$tmp/$asset" | awk '/^(\.\/)?plugin\// { found = 1 } END { exit !found }'; then
		has_plugin=1
		tar -xzf "$tmp/$asset" -C "$tmp" plugin
	else
		has_plugin=
	fi

	if [ "${TERN_KATA_PLUGIN_ONLY:-}" = 1 ]; then
		command -v tern >/dev/null 2>&1 || die "tern is not on PATH: install tern-kata's window plugin on the machine that runs Tern"
		[ -n "$has_plugin" ] || die "this release carries no window plugin: nothing was installed"
		echo "Installing the Tern window plugin (kata links, the Install tern-kata palette command, startup toasts):"
		install_plugin "$tmp/plugin" || die "could not install the tern-kata window plugin"
		return 0
	fi

	tar -xzf "$tmp/$asset" -C "$tmp" tern-kata
	mkdir -p "$dir"
	# Copy beside the target and rename over it, so upgrading a running tern-kata works.
	cp "$tmp/tern-kata" "$dir/.tern-kata.new"
	chmod 755 "$dir/.tern-kata.new"
	mv -f "$dir/.tern-kata.new" "$dir/tern-kata"
	echo "Installed $("$dir/tern-kata" --version) to $dir/tern-kata"
	# The installed binary checks this machine's kata daemon and worker.toml as `tern-kata update` does before a swap; here
	# what it finds is only printed. A release before `--preflight --text` prints nothing on stdout; stdin is /dev/null
	# because releases before 0.5 open a kata Pane for unknown arguments when run inside Tern from a terminal.
	"$dir/tern-kata" update --preflight --text </dev/null >&2 2>/dev/null || :

	case ":$PATH:" in
	*":$dir:"*) ;;
	*) echo "Add $dir to your PATH, for example: export PATH=\"$dir:\$PATH\"" ;;
	esac

	if [ -z "$has_plugin" ]; then
		echo "This release carries no Tern window plugin, so the plugin was left as it is."
	elif command -v tern >/dev/null 2>&1; then
		echo "Installing the Tern window plugin (kata links, the Install tern-kata palette command, startup toasts):"
		install_plugin "$tmp/plugin" || echo "Could not install the tern-kata window plugin; rerun this script with TERN_KATA_PLUGIN_ONLY=1 set" >&2
	else
		echo "tern is not on this machine's PATH: on the machine that shows the Tern window, install the window plugin with the plugin-only one-liner in the README (TERN_KATA_PLUGIN_ONLY=1)"
	fi

	# npx is the skills CLI's documented runner; bunx runs it the same way where only Bun is installed.
	if command -v npx >/dev/null 2>&1; then
		run="npx --yes"
		skills="npx skills add Dangerboi/tern-kata -g"
	elif command -v bunx >/dev/null 2>&1; then
		run=bunx
		skills="bunx skills add Dangerboi/tern-kata -g"
	else
		run=
		skills="npx skills add Dangerboi/tern-kata -g (or bunx)"
	fi
	# `curl | sh` gives the script the pipe as stdin, so the question goes to the terminal, and only when there is one.
	if [ -n "$run" ] && [ -t 1 ] && (: </dev/tty) 2>/dev/null; then
		printf 'Install the kata and tern-kata agent skills (%s)? [y/N] ' "$skills" >/dev/tty
		read -r answer </dev/tty || answer=
		case $answer in
		[yY]*) $run skills add Dangerboi/tern-kata -g </dev/tty || echo "Could not install the agent skills; run: $skills" >&2 ;;
		*) echo "Skipped the agent skills; to install them later: $skills" ;;
		esac
	else
		echo "Agent skills for kata and tern-kata (needs Node.js or Bun): $skills"
	fi
}

main
