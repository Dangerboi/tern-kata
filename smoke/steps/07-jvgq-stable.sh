# Clicking an Issue or a Collection redraws only what changed. The app records
# every TSP op it sends (TERN_TSP_RECORD), so a column deleted and added back,
# which flashes the window, shows even when the daemon answers in milliseconds.
kata --workspace "$tmp/workspace" create "Smoke stable first" --body "Stable first body" --json >"$tmp/stable-first.json"
stable_first=$(jq -r '.issue.uid' "$tmp/stable-first.json")
kata --workspace "$tmp/workspace" create "Smoke stable second" --body "Stable second body" --related "$(jq -r '.issue.short_id' "$tmp/stable-first.json")" --json >"$tmp/stable-second.json"
stable_second=$(jq -r '.issue.uid' "$tmp/stable-second.json")

# TSP ops sent after the first $2 lines of recording $1 that match jq filter $3.
stable_ops() { tail -n "+$(($2 + 1))" "$1" | jq -c "select(.dir == \"out\" and .verb == \"f\") | .body.ops[] | select($3)"; }
# Fails when ops $3 were sent after line $2 of recording $1.
expect_no_ops() {
  local seen
  seen=$(stable_ops "$1" "$2" "$3")
  [[ -z $seen ]] || { printf 'Unexpected TSP ops %s:\n%s\n' "$3" "$seen" >&2; return 1; }
}
# Fails unless ops $3 were sent after line $2 of recording $1.
expect_ops() { [[ -n $(stable_ops "$1" "$2" "$3") ]] || { printf 'Missing TSP ops %s\n' "$3" >&2; return 1; }; }
stable_columns='(.[0] == "add" or .[0] == "del") and (.[1] | test("^main\\.(loading|columns(\\.(collections|list(\\.issues)?|detail))?|topbar(\\.(heading|daemon|counts))?)$"))'
stable_detail='(.[0] == "add" or .[0] == "del") and (.[1] | test("^main\\.columns\\.detail\\.viewport(\\.content(\\.(heading(\\.(title|status))?|gap(-activity)?|properties|labels|body|activity))?)?$"))'
detail_reveal='.[0] == "reveal" and (.[1] | test("^main\\.columns\\.detail\\.viewport\\.content\\.(properties|labels|body|activity)(\\.[^.]+)*$"))'

# An unbound directory keeps the Collections sidebar, starting on Inbox.
mkdir -p "$tmp/stable-unbound"
quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata)"\n' "$tmp/stable-unbound" "$tmp/stable-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/stable-launch.ctl"
ctl --file "$tmp/stable-launch.ctl"
expect_tree "[data-id='main.columns.collections.inbox']" '.ok and .nodes[0]["aria-selected"] == "true"'

# Switching Collection keeps the columns; the empty Inbox list does not make it a first load.
mark=$(wc -l <"$tmp/stable-frames.jsonl")
ctl click "[data-id='main.columns.collections.all-open']"
expect_tree "[data-id='main.columns.list.issues.$stable_second']" '.ok and .count == 1'
expect_tree "[data-id='main.topbar.left.heading']" '.nodes[0].text == "All Open"'
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$stable_columns"
expect_ops "$tmp/stable-frames.jsonl" "$mark" '.[0] == "set" and .[1] == "main.topbar.left.heading"'

# Clicking another Issue replaces the detail in place and leaves the list where it is.
ctl click "[data-id='main.columns.list.issues.$stable_first']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.nodes[0].text | contains("Smoke stable first") and (contains("loading") | not)'
mark=$(wc -l <"$tmp/stable-frames.jsonl")
ctl click "[data-id='main.columns.list.issues.$stable_second']"
expect_tree "[data-id='main.columns.list.issues.$stable_second']" '.nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.nodes[0].text | contains("Smoke stable second") and (contains("loading") | not)'
expect_tree "[data-id='main.columns.detail.viewport.content.body']" '[.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Stable second body")'
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$stable_columns"
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$stable_detail"
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" '.[0] == "reveal"'
expect_ops "$tmp/stable-frames.jsonl" "$mark" '.[1] == "main.columns.detail.viewport.content.heading.title"'

# Neither a live refresh of the Issue being read nor following its link to another
# Issue reveals anything in the detail: a reveal pins Tern's list and stops the wheel (ef39).
ctl click "[data-id='main.columns.list.issues.$stable_first']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.nodes[0].text | contains("Smoke stable first") and (contains("loading") | not)'
ctl key enter
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']"
# The click's own tab reveals (19-yd2n) follow its render, so wait for the tab before marking.
expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']" '.ok and (.nodes[0].text | startswith("[ Links"))'
sleep 0.5
mark=$(wc -l <"$tmp/stable-frames.jsonl")
kata --workspace "$tmp/workspace" comment "$stable_first" --body "Stable refresh comment" --json
expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']" '.ok and (.nodes[0].text | startswith("Comments (1)"))'
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$detail_reveal"
mark=$(wc -l <"$tmp/stable-frames.jsonl")
ctl key enter
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.nodes[0].text | contains("Smoke stable second") and (contains("loading") | not)'
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$detail_reveal"
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$stable_detail"

# Leaving a populated list for an empty Collection keeps the columns too.
mark=$(wc -l <"$tmp/stable-frames.jsonl")
ctl click "[data-id='main.columns.collections.today']"
expect_tree "[data-id='main.topbar.left.heading']" '.nodes[0].text == "Today"'
expect_tree "[data-id='main.columns.list.issues.$stable_second']" '.ok and .count == 0'
expect_no_ops "$tmp/stable-frames.jsonl" "$mark" "$stable_columns"

quit_app
for fixture in stable-first stable-second; do
  uid=$(jq -r '.issue.uid' "$tmp/$fixture.json")
  short=$(jq -r '.issue.short_id' "$tmp/$fixture.json")
  kata --workspace "$tmp/workspace" delete "$uid" --force --confirm "DELETE smoke#$short" --json
done
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
