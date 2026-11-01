#!/usr/bin/env bash
# Runs the project's checks: static (gofmt, go vet and golangci-lint for Linux and Windows, the placement tests with the
# luau command line, the build and the release cross-builds), test (go test) or smoke (smoke/run.sh); no argument runs all three.
# The static checks run side by side; each prints "CHECK <name> ok" or "CHECK <name> FAIL" (after its output) when it ends.
# CHECK_OUT (default: a temporary folder, deleted on exit) receives the go test -json stream as gotest.jsonl, the built
# binaries under bin/ and each static check's output under log/.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
export CGO_ENABLED=0
if [[ -n ${CHECK_OUT:-} ]]; then
  out=$CHECK_OUT
else
  out=$(mktemp -d)
  trap 'rm -rf "$out"' EXIT
fi
mkdir -p "$out/bin" "$out/log"

check() { # check <name> <command...>: starts the check in the background and adds it to pids
  local name=$1
  shift
  (
    if "$@" >"$out/log/$name" 2>&1; then
      printf 'CHECK %s ok\n' "$name"
    else
      cat "$out/log/$name"
      printf 'CHECK %s FAIL\n' "$name"
      exit 1
    fi
  ) &
  pids+=($!)
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

placement_tests() { luau test/placement_test.luau && luau test/releases_test.luau && luau test/panecommands_test.luau; }

static() {
  local status=0 pid pids=()
  check fmt gofmt_clean
  check vet-linux go vet ./...
  check vet-windows env GOOS=windows go vet ./...
  # The two lints share golangci-lint's cache, which takes concurrent writers; the flag lifts the one-run-at-a-time lock.
  check lint-linux golangci-lint run --allow-parallel-runners ./...
  check lint-windows env GOOS=windows golangci-lint run --allow-parallel-runners ./...
  check placement placement_tests
  check build go build -o "$out/bin/tern-kata" ./cmd/tern-kata
  for os in linux darwin windows; do
    for arch in amd64 arm64; do
      check "cross-$os-$arch" cross "$os" "$arch"
    done
  done
  for pid in "${pids[@]}"; do wait "$pid" || status=1; done
  return "$status"
}

# Prints what a failed go test run reported: build errors, then the output of each failed test and package.
failures='
  [.[] | select(.Action == "fail") | [.Package, .Test]] as $failed
  | .[]
  | select(
      .Action == "build-output"
      or (.Action == "output" and ([.Package, .Test] | IN($failed[])))
    )
  | .Output
'
tests() {
  go test -json ./... >"$out/gotest.jsonl" || { jq -js "$failures" "$out/gotest.jsonl"; return 1; }
}

smoke() { ./smoke/run.sh; }

case ${1:-all} in
  static) static ;;
  test) tests ;;
  smoke) smoke ;;
  all) static && tests && smoke ;;
  *) printf 'usage: scripts/check.sh [static|test|smoke|all]\n' >&2; exit 2 ;;
esac
