#!/usr/bin/env bash
# Runs the project's checks: static (gofmt, go vet and golangci-lint for Linux and Windows, the build and the release
# cross-builds), test (go test) or smoke (smoke/run.sh); no argument runs all three.
# Each static check prints "CHECK <name> ok" or "CHECK <name> FAIL" when it ends. CHECK_OUT (default: a temporary folder)
# receives the go test -json stream as gotest.jsonl and the built binaries under bin/.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
export CGO_ENABLED=0
out=${CHECK_OUT:-$(mktemp -d)}
mkdir -p "$out/bin"

check() { # check <name> <command...>
  local name=$1
  shift
  if "$@"; then
    printf 'CHECK %s ok\n' "$name"
  else
    printf 'CHECK %s FAIL\n' "$name"
    return 1
  fi
}
gofmt_clean() {
  local unformatted
  unformatted=$(gofmt -l cmd internal)
  [[ -z $unformatted ]] || { printf '%s\n' "$unformatted" >&2; return 1; }
}
cross() { # cross <os> <arch>
  local bin=tern-kata
  [[ $1 != windows ]] || bin=tern-kata.exe
  GOOS=$1 GOARCH=$2 go build -trimpath -o "$out/bin/$1_$2/$bin" ./cmd/tern-kata
}

static() {
  local status=0
  check fmt gofmt_clean || status=1
  check vet-linux go vet ./... || status=1
  check vet-windows env GOOS=windows go vet ./... || status=1
  check lint-linux golangci-lint run ./... || status=1
  check lint-windows env GOOS=windows golangci-lint run ./... || status=1
  check build go build -o "$out/bin/tern-kata" ./cmd/tern-kata || status=1
  for os in linux darwin windows; do
    for arch in amd64 arm64; do
      check "cross-$os-$arch" cross "$os" "$arch" || status=1
    done
  done
  return "$status"
}

tests() {
  go test -json ./... >"$out/gotest.jsonl" || { jq -r 'select(.Action == "fail" and .Test == null) | "FAIL \(.Package)"' "$out/gotest.jsonl"; return 1; }
}

smoke() { ./smoke/run.sh; }

case ${1:-all} in
  static) static ;;
  test) tests ;;
  smoke) smoke ;;
  all) static && tests && smoke ;;
  *) printf 'usage: scripts/check.sh [static|test|smoke|all]\n' >&2; exit 2 ;;
esac
