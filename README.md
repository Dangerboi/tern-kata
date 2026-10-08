# tern-kata

Native kata Issues in a Tern Pane. Requires Go 1.27+, kata v0.18.0+ on `PATH`, and Tern with Surface Protocol support. Without Tern support, arguments pass unchanged to `kata tui`.

```sh
CGO_ENABLED=0 go build -o tern-kata .
CGO_ENABLED=0 go install github.com/Dangerboi/tern-kata@latest
CGO_ENABLED=0 go test ./...
./smoke/run.sh
```

Run `tern-kata` inside Tern in a directory bound with `kata init`. Use `j`/`k`, `g`/`G`, and `q`/`ctrl+c`; the footer comes from `model/keymap.go`.

The smoke runner needs `kata`, `tern` and `jq` on `PATH`. It isolates both instances, seeds three open Issues in Project `smoke`, and runs `smoke/steps/*` in filename order: `.ctl` scenarios or sourced `.sh` assertions using `ctl` and `expect_tree`. Add a step file for each feature; leave the app in the Issue list for the next step. Use `plugins expect` for native text, but `expect_tree` for list labels (Tern's text search excludes them). Failed runs retain their temporary artifacts and exit non-zero.

Views use Tern's native typography, spacing and selection accent for readable Issue rows, following the host theme without a palette or added animation (energy/rhythm/motion 1/1/1). Model Msg/Cmd values are extended in per-feature files; `model/keymap.go` remains the single source for key handling and hints.
