# Development

```sh
CGO_ENABLED=0 go build -o tern-kata ./cmd/tern-kata
CGO_ENABLED=0 go test ./...
./smoke/run.sh      # end-to-end against a headless Tern and a throwaway kata daemon
tern plugin link .  # use this checkout as the window plugin
tern plugin reload  # exits 1 if a plugin fails to load; window failures show only as a toast
```

Every push to `main` and every pull request runs `.github/workflows/ci.yml`: gofmt, `go vet` for Linux and for Windows, golangci-lint (`.golangci.yml`) for Linux and for Windows, and `go test ./...`. Run `golangci-lint run ./...` locally with a golangci-lint built by Go 1.27 or newer; an older build cannot load this toolchain's standard library. The smoke suite needs Tern and a kata daemon, so it runs only locally.

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
