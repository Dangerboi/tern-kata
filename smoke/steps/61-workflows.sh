# The Workflows Page (4) lists the Project's workflows. Enter on a selected one shows its steps beside the list, and a double
# click on a workflow that is not selected does the same for that workflow. A workflow with more steps than the Pane shows
# scrolls inside its card: the top bar stays put and the card ends above the footer. Own workflows; it cleans up.
wf_dir="$tmp/workspace"
wf_make() { # uid name command
  jq -n --arg name "$2" --arg command "$3" '{name: $name, definition: {version: 1, steps: [{key: "only", kind: "command", command: $command}]}}' >"$tmp/workflows-$1.json"
  kata --workspace "$wf_dir" cron workflow create --uid "$1" --file "$tmp/workflows-$1.json" --json >/dev/null
}
wf_first=01H5MK0000000000000000W001
wf_second=01H5MK0000000000000000W002
wf_make "$wf_first" "Smoke first workflow" "echo first-step"
wf_make "$wf_second" "Smoke second workflow" "echo second-step"

ctl key 4
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_first']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_second']" '.ok and .count == 1'
# A single click only selects: no steps are shown.
ctl click "[data-id='main.columns.list.page.names.rows.$wf_first']"
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_first']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id\$='.steps-title']" '.count == 0'
# Enter shows the steps of the selected workflow.
ctl key enter
expect_tree "[data-id\$='.steps-title']" '.ok and .count == 1'
expect_tree "[data-id^='main.columns.list.page.inspector']" '.ok and ([.nodes[] | (.text // "")] | any(contains("echo first-step")))'
ctl key escape
expect_tree "[data-id\$='.steps-title']" '.count == 0'
# A double click on the other workflow selects it and shows its steps, as Enter does.
ctl dblclick "[data-id='main.columns.list.page.names.rows.$wf_second']"
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_second']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id\$='.steps-title']" '.ok and .count == 1'
expect_tree "[data-id^='main.columns.list.page.inspector']" '.ok and ([.nodes[] | (.text // "")] | any(contains("echo second-step")))'
ctl key escape
expect_tree "[data-id\$='.steps-title']" '.count == 0'
ctl key 1

# A long workflow: the inspector card stays inside the Pane and its viewport scrolls to the last step.
wf_long=01H5MK0000000000000000W003
jq -n '{name: "Smoke long workflow", definition: {version: 1, steps: [range(60) | {key: "step\(.)", kind: "command", command: "echo long-step-\(.)"}]}}' >"$tmp/workflows-long.json"
kata --workspace "$wf_dir" cron workflow create --uid "$wf_long" --file "$tmp/workflows-long.json" --json >/dev/null
ctl key 4
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_long']" '.ok and .count == 1'
wf_bar=$(ctl tree "[data-id='main.topbar']" | jq -c '.nodes[0].rect')
ctl dblclick "[data-id='main.columns.list.page.names.rows.$wf_long']"
expect_tree "[data-id='main.columns.list.page.inspector.viewport.content']" '.ok and .count == 1'
ctl tree "[data-id='dock.hints']" | jq '.nodes[0].rect[1]' >"$tmp/workflows-dock-top.txt"
ctl tree "[data-id='main.columns.list.page.inspector'],[data-id='main.columns.list.page.names']" >"$tmp/workflows-cards.json"
jq -e --slurpfile dock "$tmp/workflows-dock-top.txt" '.count == 2 and all(.nodes[].rect; .[1] + .[3] <= $dock[0])' "$tmp/workflows-cards.json" || { printf 'A Workflows card runs past the footer\n' >&2; return 1; }
wf_below() { ctl tree "[data-id='main.columns.list.page.inspector.viewport'],[data-id='main.columns.list.page.inspector.viewport.content']" | jq -r '.nodes | (.[1].rect[1] + .[1].rect[3]) - (.[0].rect[1] + .[0].rect[3]) | floor'; }
[[ $(wf_below) -gt 100 ]] || { printf 'The long workflow does not overflow its inspector viewport\n' >&2; return 1; }
ctl hover "[data-id='main.columns.list.page.inspector.viewport']" >/dev/null
for _ in $(seq 40); do ctl wheel 0 300 >/dev/null; done
for ((attempt = 0; attempt < 40; attempt++)); do [[ $(wf_below) -le 1 ]] && break; sleep 0.1; done
[[ $(wf_below) -le 1 ]] || { printf 'The inspector viewport does not scroll to its end\n' >&2; return 1; }
[[ $(ctl tree "[data-id='main.topbar']" | jq -c '.nodes[0].rect') == "$wf_bar" ]] || { printf 'The top bar moved while the inspector scrolled\n' >&2; return 1; }
ctl key escape
ctl key 1
for wf_uid in "$wf_first" "$wf_second" "$wf_long"; do
  kata --workspace "$wf_dir" cron workflow show "$wf_uid" --json >"$tmp/workflows-show.json"
  kata --workspace "$wf_dir" cron workflow delete "$wf_uid" --expected-event-uid "$(jq -r '[.. | .definition_event_uid? // empty][0]' "$tmp/workflows-show.json")" --json >/dev/null
done
