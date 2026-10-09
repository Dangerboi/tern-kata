# The Activity tabs: Comments, Events and Links with their counts, one tab's rows at a time. tab and
# shift+tab cycle them, a click on a tab name selects it, the tab stays when another Issue opens, and j/k
# move the row cursor without making the detail reveal anything (Tern pins a reveal's target and kills the
# wheel), so the wheel and the keys still scroll the detail afterwards. Own Project and Issues; it cleans up.
at_rects() { ctl tree "[data-id='main.columns.detail.viewport'],[data-id='main.columns.detail.viewport.content']" | jq -c '[.nodes[].rect]'; }
# How far the content is scrolled and how much is left below the viewport's bottom edge, in px.
at_above() { at_rects | jq -r '.[1][1] - .[0][1] | -floor'; }
at_below() { at_rects | jq -r '(.[1][1] + .[1][3]) - (.[0][1] + .[0][3]) | floor'; }
at_wait() { # <label> <jq test on above/below as $a/$b>
  local a b
  for ((attempt = 0; attempt < 40; attempt++)); do
    a=$(at_above); b=$(at_below)
    jq -en --argjson a "$a" --argjson b "$b" "$2" >/dev/null && return 0
    sleep 0.1
  done
  printf '%s: scrolled %s px, %s px left below\n' "$1" "$a" "$b" >&2
  return 1
}
at_wheel() { ctl hover "[data-id='main.columns.detail.viewport']" >/dev/null; for _ in $(seq "$2"); do ctl wheel 0 "$1" >/dev/null; done; }
at_keys() { for _ in $(seq "$2"); do ctl key "$1" >/dev/null; done; }


at_dir="$tmp/tabs-workspace"
mkdir -p "$at_dir" "$tmp/tabs-unbound"
kata --workspace "$at_dir" init --project tabs-detail --json >/dev/null
at_project=$(kata --workspace "$at_dir" projects list --json | jq -er '.projects[] | select(.name == "tabs-detail") | .uid')
kata --workspace "$at_dir" create "Tabs peer" --json >"$tmp/tabs-peer.json"
at_body=$(for line in $(seq 40); do printf 'Body line %s\n\n' "$line"; done)
kata --workspace "$at_dir" create "Tabs main" --body "$at_body" --blocked-by "$(jq -r '.issue.short_id' "$tmp/tabs-peer.json")" --json >"$tmp/tabs-main.json"
at_main=$(jq -r '.issue.uid' "$tmp/tabs-main.json")
at_peer=$(jq -r '.issue.uid' "$tmp/tabs-peer.json")
for n in 1 2; do kata --workspace "$at_dir" comment "$(jq -r '.issue.short_id' "$tmp/tabs-main.json")" --body "Tabs comment $n" --json >/dev/null; done

quit_app
rm -f "$tmp/tabs-frames.jsonl"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata)"\n' "$tmp/tabs-unbound" "$tmp/tabs-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/tabs-launch.ctl"
ctl --file "$tmp/tabs-launch.ctl"
expect_tree "[data-id='main.columns.collections.project.$at_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$at_project']"
expect_tree "[data-id\$='.$at_main']" '.ok and .count == 1'
ctl click "[data-id\$='.$at_main']"
at_tab="main.columns.detail.viewport.content.activity.tabs.tab"
at_rows="[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.heading']"
at_label() { expect_tree "[data-id='$at_tab$1']" ".ok and (.nodes[0].text | $2)"; }
ctl key enter

# Comments (2) is the default, bracketed; the Events and Links tabs carry their counts.
at_label 0 'startswith("[ Comments (2) ]")'
at_label 1 'test("^Events \\([0-9]+\\)$")'
at_label 2 'startswith("Links (1)")'
expect_tree "$at_rows" '.ok and .count == 2 and ([.nodes[].text] | all(contains("dangerboi")))'

# tab and shift+tab cycle Comments -> Events -> Links -> Comments.
ctl key Tab
at_label 1 'startswith("[ Events (")'
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']" '.ok and .count == 0'
ctl key Tab
at_label 2 'startswith("[ Links (1) ]")'
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.'][data-id\$='.jump']" '.ok and .count == 1'
ctl key Tab
at_label 0 'startswith("[ Comments (2) ]")'
ctl key shift+Tab
at_label 2 'startswith("[ Links (1) ]")'
ctl key Tab

# j/k move the row cursor, and the wheel, ↓ and pgdn still scroll the detail afterwards.
at_keys j 2
at_keys k 1
at_wheel 300 12
at_wait "the wheel reaches the end after j/k" '$b <= 1'
at_wheel -300 12
at_wait "the wheel returns to the top after j/k" '$a == 0'
at_keys Down 3
at_wait "↓ scrolls after j/k" '$a > 0'
at_keys PageUp 12
at_wait "pgup returns to the top after j/k" '$a == 0'
jq -c 'select(.dir == "out" and .verb == "f") | .body.ops[] | select(.[0] == "reveal" and (.[1] | startswith("main.columns.detail")))' "$tmp/tabs-frames.jsonl" >"$tmp/tabs-reveals.json"
[[ ! -s $tmp/tabs-reveals.json ]] || { printf 'Reveal inside the detail for a key: %s\n' "$(<"$tmp/tabs-reveals.json")" >&2; return 1; }

# A click on a tab name selects it, and the tab stays when another Issue opens.
at_wheel 300 12
at_wait "the tabs are in view" '$b <= 1'
ctl click "[data-id='${at_tab}1']"
at_label 1 'startswith("[ Events (")'
ctl click "[data-id\$='.$at_peer']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Tabs peer"))'
at_label 1 'startswith("[ Events (")'
at_label 0 'startswith("Comments (0)")'

quit_app
kata projects remove tabs-detail --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
