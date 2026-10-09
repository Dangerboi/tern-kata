# Browsing a long list scrolls only the Issue rows; the top bar and header stay.
# The recorded TSP ops (TERN_TSP_RECORD) show what moved while j/k walked the list.
pinned_shorts=()
for n in $(seq -w 1 40); do
  pinned_shorts+=("$(kata --workspace "$tmp/workspace" create "Pinned row $n" --json | jq -r '.issue.short_id')")
done
quit_app
rm -f "$tmp/pinned-frames.jsonl"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata)"\n' "$tmp/workspace" "$tmp/pinned-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/pinned-launch.ctl"
ctl --file "$tmp/pinned-launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 40'
ctl key Home
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[0]["aria-selected"] == "true"'
ctl tree "[data-id='main.topbar'],[data-id='main.columns.list.header']" | jq -c '[.nodes[].rect]' >"$tmp/pinned-rects-before.json"
mark=$(wc -l <"$tmp/pinned-frames.jsonl")

# Page Down moves the selection by a page of rows (the nearest reveal keeps it on screen) and the bar stays put.
ctl key PageDown
ctl key PageDown
expect_tree "[data-id^='main.columns.list.issues.01']" '(.nodes | map(.["aria-selected"] == "true") | index(true)) == 20'
ctl tree "[data-id='main.topbar'],[data-id='main.columns.list.header']" | jq -c '[.nodes[].rect]' | cmp - "$tmp/pinned-rects-before.json" || { printf 'Top bar or header moved on Page Down\n' >&2; return 1; }
ctl key PageUp
ctl key PageUp
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[0]["aria-selected"] == "true"'

for i in $(seq 45); do ctl key j; done
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[-1]["aria-selected"] == "true"'
ctl tree "[data-id='main.topbar'],[data-id='main.columns.list.header']" | jq -c '[.nodes[].rect]' >"$tmp/pinned-rects-after.json"
# At the end the selected row is on screen, above the footer: the list scrolled, not the Pane.
ctl tree "[data-id='dock.hints']" | jq '.nodes[0].rect[1]' >"$tmp/pinned-dock-top.txt"
ctl tree "[data-id^='main.columns.list.issues.01'][aria-selected='true']" >"$tmp/pinned-selected.json"
jq -e --slurpfile dock "$tmp/pinned-dock-top.txt" '.count == 1 and (.nodes[0].rect | .[1] >= 0 and .[1] + .[3] <= $dock[0])' "$tmp/pinned-selected.json"
for i in $(seq 45); do ctl key k; done
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[0]["aria-selected"] == "true"'

pinned_ops() { tail -n "+$(($mark + 1))" "$tmp/pinned-frames.jsonl" | jq -c "select(.dir == \"out\" and .verb == \"f\") | .body.ops[] | select($1)"; }
[[ -z $(pinned_ops '(.[1] | test("^main\\.topbar(\\.(heading|daemon|counts))?$|^main\\.columns\\.list\\.header"))') ]] || { printf 'Top bar or header node changed while browsing\n' >&2; return 1; }
[[ -n $(pinned_ops '.[0] == "reveal"') ]] || { printf 'No reveal sent while browsing\n' >&2; return 1; }
[[ -z $(pinned_ops '.[0] == "reveal" and (.[1] | test("^main\\.columns\\.list\\.issues\\.0")) == false') ]] || { printf 'Reveal on a node other than an Issue row\n' >&2; return 1; }
[[ -z $(pinned_ops '.[0] == "reveal" and .[2] != "nearest"') ]] || { printf 'Reveal other than nearest\n' >&2; return 1; }
[[ -z $(pinned_ops '.[0] == "scroll" and .[1] != "main.columns.detail.viewport.content"') ]] || { printf 'Scroll op on a node other than the detail\n' >&2; return 1; }
cmp "$tmp/pinned-rects-before.json" "$tmp/pinned-rects-after.json" || { printf 'Top bar or header moved while browsing\n' >&2; return 1; }

quit_app
for short in "${pinned_shorts[@]}"; do
  kata --workspace "$tmp/workspace" delete "smoke#$short" --force --confirm "DELETE smoke#$short" --json >/dev/null
done
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
