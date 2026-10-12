quit_app
stop_live_daemon
kata daemon start --listen 127.0.0.1:0
remote_upstream=$(kata daemon locate --json | jq -er '.request_base_url')
remote_workspace="$tmp/remote-client"
mkdir -p "$remote_workspace/nested" "$tmp/remote-missing"
git init -q "$remote_workspace"
git -C "$remote_workspace" remote add origin git@github.com:tern-kata/review-remote.git
env KATA_SERVER="$remote_upstream" kata --workspace "$remote_workspace" init --project remote-smoke --json >"$tmp/remote-project.json"
remote_project_uid=$(jq -er '.project.uid' "$tmp/remote-project.json")
env KATA_SERVER="$remote_upstream" kata --workspace "$remote_workspace" create 'Remote local-binding Issue' --json >"$tmp/remote-issue.json"
remote_issue_uid=$(jq -er '.issue.uid' "$tmp/remote-issue.json")
env KATA_SERVER="$remote_upstream" kata --workspace "$tmp" projects rename remote-smoke remote-renamed --json
printf 'version = 1\n[project]\nname = "remote-smoke"\n' >"$remote_workspace/.kata.toml"

python3 "$repo/smoke/remote_proxy.py" "$remote_upstream" "$tmp/remote-resolve.jsonl" >"$tmp/proxy.url" 2>"$tmp/proxy.log" &
proxy_pid=$!
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ ! -s "$tmp/proxy.url" ]] || break
  kill -0 "$proxy_pid"
  sleep 0.1
done
[[ -s "$tmp/proxy.url" ]]
remote_proxy=$(<"$tmp/proxy.url")
printf 'ready\nrun "(cd %s && env -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s KATA_SERVER=%s %s/tern-kata)"\n' "$remote_workspace/nested" "$KATA_HOME" "$remote_proxy" "$tmp" >"$tmp/remote-launch.ctl"
ctl --file "$tmp/remote-launch.ctl"
expect_tree "[data-id='main.topbar.left.heading']" '.ok and .count == 1 and .nodes[0].text == "Project: remote-renamed"'
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.list.issues.$remote_issue_uid']" '.ok and .count == 1 and (.nodes[0].text | contains("Remote local-binding Issue"))'
jq -se 'length > 0 and all(.[]; (has("start_path") | not)) and any(.[]; .name == "remote-smoke" and .alias.identity == "github.com/tern-kata/review-remote" and .alias.kind == "git")' "$tmp/remote-resolve.jsonl" >/dev/null
quit_app

printf 'version = 1\n[project]\nname = "remote-missing"\n' >"$tmp/remote-missing/.kata.toml"
printf 'ready\nrun "(cd %s && env -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s KATA_SERVER=%s %s/tern-kata)"\n' "$tmp/remote-missing" "$KATA_HOME" "$remote_proxy" "$tmp" >"$tmp/remote-missing.ctl"
ctl --file "$tmp/remote-missing.ctl"
expect_tree "[data-id='main.error']" '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("project remote-missing is not registered") and contains("run \"kata init\" in this workspace") and (contains("unexpected status code") | not)'
quit_app

printf 'version = 1\n[project\n' >"$tmp/remote-missing/.kata.toml"
printf 'ready\nrun "(cd %s && env -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s KATA_SERVER=%s %s/tern-kata)"\n' "$tmp/remote-missing" "$KATA_HOME" "$remote_proxy" "$tmp" >"$tmp/remote-malformed.ctl"
ctl --file "$tmp/remote-malformed.ctl"
expect_tree "[data-id='main.error']" '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("parse ") and contains("/remote-missing/.kata.toml:")'
quit_app
jq -se 'all(.[]; (has("start_path") | not)) and any(.[]; .name == "remote-missing")' "$tmp/remote-resolve.jsonl" >/dev/null
kill "$proxy_pid"
wait "$proxy_pid" 2>/dev/null || true
proxy_pid=
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
