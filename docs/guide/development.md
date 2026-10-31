# Development

```sh
CGO_ENABLED=0 go build -o tern-kata ./cmd/tern-kata
CGO_ENABLED=0 go test ./...
./smoke/run.sh               # end-to-end against headless Tern servers and throwaway kata daemons
scripts/check.sh             # everything: static (gofmt, vet, golangci-lint, builds), test or smoke alone with that argument
tern plugin link .           # use this checkout as the window plugin
tern plugin reload tern-kata # exits 1 if it fails to load; window failures show only as a toast
```

Every push to `main` and every pull request runs `.github/workflows/ci.yml` as two jobs side by side: `static` (`scripts/check.sh static`: gofmt, `go vet` and golangci-lint (`.golangci.yml`) for Linux and for Windows, the build and the release cross-builds, all at once, with golangci-lint v2.15.0 installed by `go install`) and `test` (`go test ./...`). Run `scripts/check.sh static` locally for the same checks; `scripts/check.sh test` prints the output of failing tests, and without `CHECK_OUT` the script deletes its temporary output folder on exit. Run `golangci-lint run ./...` locally with a golangci-lint built by Go 1.27 or newer; an older build cannot load this toolchain's standard library. The smoke suite needs Tern and a kata daemon, so it runs only locally.

`./smoke/run.sh` with no arguments runs the steps as parallel shards, each on its own headless Tern and kata daemon: five runs of consecutive UI steps (cut at 22, 32, 45 and 58), one run each for the steps that wait on kata's daemon clock (51, 60, 62), and `plugin.sh` and `supervision.sh`. A step that needs state left by an earlier step must stay in that step's shard. `./smoke/run.sh STEP…` runs the given steps in order on one server, then `plugin.sh` and `supervision.sh`.

## Releases

Pushing a `vX.Y.Z` tag runs `.github/workflows/release.yml` (vet, test, then publish), with the tag's `CHANGELOG.md` section as the release notes. Check it locally before tagging:

```sh
scripts/release-notes.sh vX.Y.Z   # prints the notes; fails unless CHANGELOG.md has a [X.Y.Z] section
scripts/release.sh vX.Y.Z         # writes dist/ (gitignored); fails unless plugin.toml's version is X.Y.Z and the notes exist; publishes nothing
```

It builds `tern-kata_<os>_<arch>.tar.gz` (`.zip` for `windows`) for `linux`, `darwin` and `windows` on `amd64` and `arm64`, plus `checksums.txt` in `sha256sum` format.

A release build sets the version with `-ldflags "-X main.version=vX.Y.Z"`. Without it, `tern-kata --version` reports the Go module version (`go install …@vX.Y.Z`), or `dev` for an untagged build.

## Type-checking the plugin

`window.luau` and `host.luau` are `--!strict`. To type-check them against the installed Tern's API (needs [luau-lsp](https://github.com/JohnnyMorganz/luau-lsp)):

```sh
tern plugin types .  # writes tern.d.luau (gitignored; regenerate per Tern version)
luau-lsp analyze --platform=standard --definitions=@tern=tern.d.luau --ignore=tern.d.luau window.luau host.luau
```

## Design records

The [ADRs](../adr) record the design decisions behind the plugin and the worker.
