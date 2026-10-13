#!/bin/sh
# Installs the latest tern-kata release for Linux or macOS: curl -fsSL https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.sh | sh
# TERN_KATA_BASE_URL replaces the download location (default: the latest GitHub release), TERN_KATA_INSTALL_DIR the install folder.
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

	tar -xzf "$tmp/$asset" -C "$tmp" tern-kata
	mkdir -p "$dir"
	# Copy beside the target and rename over it, so upgrading a running tern-kata works.
	cp "$tmp/tern-kata" "$dir/.tern-kata.new"
	chmod 755 "$dir/.tern-kata.new"
	mv -f "$dir/.tern-kata.new" "$dir/tern-kata"
	echo "Installed $("$dir/tern-kata" --version) to $dir/tern-kata"

	case ":$PATH:" in
	*":$dir:"*) ;;
	*) echo "Add $dir to your PATH, for example: export PATH=\"$dir:\$PATH\"" ;;
	esac

	if command -v tern >/dev/null 2>&1; then
		echo "Installing the Tern window plugin (kata links, the Install tern-kata palette command, startup toasts):"
		tern plugin install github.com/Dangerboi/tern-kata --force || echo "Could not install the tern-kata window plugin; run: tern plugin install github.com/Dangerboi/tern-kata --force" >&2
	else
		echo "tern is not on this machine's PATH: install the window plugin on the machine that shows the Tern window: tern plugin install github.com/Dangerboi/tern-kata"
	fi
}

main
