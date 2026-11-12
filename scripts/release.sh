#!/bin/sh
# Builds the release archives and checksums.txt (archives only) and version.txt into DIST (default dist/) without publishing.
# Usage: scripts/release.sh vX.Y.Z [DIST]
set -eu

tag=${1:?usage: scripts/release.sh vX.Y.Z [DIST]}
dist=${2:-dist}
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

manifest=$(sed -n 's/^version *= *"\(.*\)"/\1/p' plugin.toml)
if [ "$manifest" != "${tag#v}" ]; then
	echo "plugin.toml version \"$manifest\" does not match tag \"$tag\"" >&2
	exit 1
fi
scripts/release-notes.sh "$tag" >/dev/null

rm -rf "$dist"
mkdir -p "$dist"
dist=$(cd "$dist" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

# The plugin folder every archive carries; `tern plugin install <dir>` installs it. A require("./name") in a plugin file
# needs name.luau beside it, or the installed plugin would fail to load.
plugin_files="plugin.toml window.luau host.luau placement.luau releases.luau panecommands.luau waitingtext.luau waitingagents.luau widgets.luau workersummary.luau workerwidget.luau"
mkdir "$stage/plugin"
for f in $plugin_files; do
	cp "$f" "$stage/plugin/"
done
for f in "$stage"/plugin/*.luau; do
	for name in $(sed -n 's/.*require("\.\/\([^"]*\)").*/\1/p' "$f"); do
		if [ ! -f "$stage/plugin/$name.luau" ]; then
			echo "$(basename "$f") requires ./$name, but plugin/ has no $name.luau; add it to plugin_files in scripts/release.sh" >&2
			exit 1
		fi
	done
done

# The six builds run side by side; a failed one fails the script at its wait.
pids=
for os in linux darwin windows; do
	for arch in amd64 arm64; do
		bin=tern-kata
		[ "$os" = windows ] && bin=tern-kata.exe
		mkdir -p "$stage/${os}_$arch"
		CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-X main.version=$tag" -o "$stage/${os}_$arch/$bin" ./cmd/tern-kata &
		pids="$pids $!"
	done
done
for pid in $pids; do
	wait "$pid"
done

for os in linux darwin windows; do
	for arch in amd64 arm64; do
		bin=tern-kata
		[ "$os" = windows ] && bin=tern-kata.exe
		dir=$stage/${os}_$arch
		cp LICENSE "$dir/"
		cp -R "$stage/plugin" "$dir/plugin"
		if [ "$os" = windows ]; then
			(cd "$dir" && zip -qr "$dist/tern-kata_${os}_$arch.zip" "$bin" LICENSE plugin)
		else
			tar -C "$dir" -czf "$dist/tern-kata_${os}_$arch.tar.gz" "$bin" LICENSE plugin
		fi
	done
done

printf '%s\n' "$manifest" >"$dist/version.txt"
(cd "$dist" && sha256sum tern-kata_* > checksums.txt)
