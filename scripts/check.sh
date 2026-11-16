#!/usr/bin/env bash
# Runs the project's checks: static (gofmt, go vet and golangci-lint for Linux and Windows, every Luau test (test/*_test.luau) with the
# luau command line, the build and the release cross-builds), test (go test, and bun test for the omp extension) or smoke
# (smoke/run.sh); no argument runs all three.
# The static checks run side by side; each prints "CHECK <name> ok" or "CHECK <name> FAIL" (after its output) when it ends.
# Every mode ends with one line: "CHECKS ok", or "CHECKS FAIL: <names>" (static: the failed checks; test, smoke, all: the failed group).
# CHECK_OUT (default: a temporary folder, deleted on exit) receives the go test -json stream as gotest.jsonl and each static
# check's output under log/.
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
mkdir -p "$out/log"

check() { # check <name> <command...>: starts the check in the background and adds it to pids
  local name=$1
  shift
  (
    if "$@" >"$out/log/$name" 2>&1; then
      printf 'CHECK %s ok\n' "$name"
    else
      cat "$out/log/$name"
      printf 'CHECK %s FAIL\n' "$name"
      : >"$out/log/$name.fail"
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

luau_tests() {
  local t
  for t in test/*_test.luau; do luau "$t" || return 1; done
}

static() {
  local status=0 pid pids=()
  rm -f "$out"/log/*.fail # a kept CHECK_OUT still holds the markers of earlier runs
  check fmt gofmt_clean
  check vet-linux go vet ./...
  check vet-windows env GOOS=windows go vet ./...
  # The two lints share golangci-lint's cache, which takes concurrent writers; the flag lifts the one-run-at-a-time lock.
  check lint-linux golangci-lint run --allow-parallel-runners ./...
  check lint-windows env GOOS=windows golangci-lint run --allow-parallel-runners ./...
  check luau-tests luau_tests
  check build go build -o /dev/null ./cmd/tern-kata
  for os in linux darwin windows; do
    for arch in amd64 arm64; do
      check "cross-$os-$arch" env GOOS="$os" GOARCH="$arch" go build -trimpath -o /dev/null ./cmd/tern-kata
    done
  done
  for pid in "${pids[@]}"; do wait "$pid" || status=1; done
  return "$status"
}

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
  bun test internal/data/ompext
}

smoke() { ./smoke/run.sh; }

failed=()
group() { # group <name> <function>: runs it, recording the group's name when it fails
  "$2" || { failed+=("$1"); return 1; }
}
case ${1:-all} in
  static) group static static || true ;;
  test) group test tests || true ;;
  smoke) group smoke smoke || true ;;
  all) group static static && group test tests && group smoke smoke || true ;;
  *) printf 'usage: scripts/check.sh [static|test|smoke|all]\n' >&2; exit 2 ;;
esac
if (( ${#failed[@]} )); then
  if [[ ${1:-all} == static ]]; then
    names=
    for marker in "$out"/log/*.fail; do
      [[ ! -e $marker ]] || names+="${names:+ }$(basename "$marker" .fail)"
    done
    failed=("${names:-static}")
  fi
  printf 'CHECKS FAIL: %s\n' "${failed[*]}"
  exit 1
fi
printf 'CHECKS ok\n'
