#!/bin/sh
# Builds the release archives and checksums.txt into DIST (default dist/) without publishing.
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

rm -rf "$dist"
mkdir -p "$dist"
dist=$(cd "$dist" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

for os in linux darwin windows; do
	for arch in amd64 arm64; do
		bin=tern-kata
		[ "$os" = windows ] && bin=tern-kata.exe
		dir=$stage/${os}_$arch
		mkdir -p "$dir"
		CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-X main.version=$tag" -o "$dir/$bin" ./cmd/tern-kata
		cp LICENSE "$dir/"
		if [ "$os" = windows ]; then
			(cd "$dir" && zip -q "$dist/tern-kata_${os}_$arch.zip" "$bin" LICENSE)
		else
			tar -C "$dir" -czf "$dist/tern-kata_${os}_$arch.tar.gz" "$bin" LICENSE
		fi
	done
done

(cd "$dist" && sha256sum tern-kata_* > checksums.txt)
