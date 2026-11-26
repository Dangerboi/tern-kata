# The Workflows Page (4) lists the Project's workflows with the selected one's steps beside the list. A workflow with more steps than the Pane shows
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
# The selected workflow's steps sit beside the list from the first frame, with no Enter: a wide Pane splits like the Issues Page.
expect_tree "[data-id\$='.steps-title']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.page.names.rows.$wf_first']"
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_first']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id^='main.columns.list.page.inspector']" '.ok and ([.nodes[] | tostring] | any(contains("echo first-step")))'
# A click on the other workflow shows its steps in the same place.
ctl click "[data-id='main.columns.list.page.names.rows.$wf_second']"
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_second']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id^='main.columns.list.page.inspector']" '.ok and ([.nodes[] | tostring] | any(contains("echo second-step")))'

# The Workflow card is a form: name, about, input, one step at a time, and buttons to add, remove and move steps. A step added
# from the card is saved through kata with the rest.
ctl key e
expect_tree "[data-id='layer.workflow']" '.ok and .count == 1'
expect_tree "[data-id='layer.workflow.name']" '.ok and .count == 1'
expect_tree "[data-id='layer.workflow.step']" '.ok and .count == 1'
expect_tree "[data-id='layer.workflow.body']" '.ok and .count == 1'
ctl click "[data-id='layer.workflow.steps-head.add']"
expect_tree "[data-id='layer.workflow.key']" '.ok and .count == 1 and ([.nodes[] | tostring] | any(contains("step")))'
ctl click "[data-id='layer.workflow.foot.save']"
expect_tree "[data-id='layer.workflow.foot.error']" '.ok and .count == 1 and (.nodes[0].text | contains("Cannot save the workflow"))'
ctl click "[data-id='layer.workflow.steps-head.remove']"
ctl click "[data-id='layer.workflow.foot.cancel']"
expect_tree "[data-id='layer.workflow']" '.count == 0'
ctl click "[data-id='main.columns.list.page.names.rows.$wf_first']"
ctl key 1

# A long workflow: the inspector card stays inside the Pane and its viewport scrolls to the last step.
wf_long=01H5MK0000000000000000W003
jq -n '{name: "Smoke long workflow", definition: {version: 1, steps: [range(60) | {key: "step\(.)", kind: "command", command: "echo long-step-\(.)"}]}}' >"$tmp/workflows-long.json"
kata --workspace "$wf_dir" cron workflow create --uid "$wf_long" --file "$tmp/workflows-long.json" --json >/dev/null
ctl key 4
expect_tree "[data-id='main.columns.list.page.names.rows.$wf_long']" '.ok and .count == 1'
wf_bar=$(ctl tree "[data-id='main.topbar']" | jq -c '.nodes[0].rect')
ctl click "[data-id='main.columns.list.page.names.rows.$wf_long']"
expect_tree "[data-id='main.columns.list.page.inspector.viewport.content']" '.ok and .count == 1'
ctl tree "[data-id='dock.hints']" | jq '.nodes[0].rect[1]' >"$tmp/workflows-dock-top.txt"
ctl tree "[data-id='main.columns.list.page.inspector'],[data-id='main.columns.list.page.names']" >"$tmp/workflows-cards.json"
jq -e --slurpfile dock "$tmp/workflows-dock-top.txt" '.count == 2 and all(.nodes[].rect; .[1] + .[3] <= $dock[0])' "$tmp/workflows-cards.json" || { printf 'A Workflows card runs past the footer\n' >&2; return 1; }
wf_below() { ctl tree "[data-id='main.columns.list.page.inspector.viewport'],[data-id='main.columns.list.page.inspector.viewport.content']" | jq -r '.nodes | (.[1].rect[1] + .[1].rect[3]) - (.[0].rect[1] + .[0].rect[3]) | floor'; }
# The 60 steps are laid out over a few frames after the content node appears.
wf_overflows() { [[ $(wf_below) -gt 100 ]]; }
wait_for wf_overflows || { printf 'The long workflow does not overflow its inspector viewport\n' >&2; return 1; }
ctl hover "[data-id='main.columns.list.page.inspector.viewport']" >/dev/null
for _ in $(seq 40); do ctl wheel 0 300 >/dev/null; done
wf_at_end() { [[ $(wf_below) -le 1 ]]; }
wait_for wf_at_end || { printf 'The inspector viewport does not scroll to its end\n' >&2; return 1; }
[[ $(ctl tree "[data-id='main.topbar']" | jq -c '.nodes[0].rect') == "$wf_bar" ]] || { printf 'The top bar moved while the inspector scrolled\n' >&2; return 1; }
ctl key 1
for wf_uid in "$wf_first" "$wf_second" "$wf_long"; do
  kata --workspace "$wf_dir" cron workflow show "$wf_uid" --json >"$tmp/workflows-show.json"
  kata --workspace "$wf_dir" cron workflow delete "$wf_uid" --expected-event-uid "$(jq -r '[.. | .definition_event_uid? // empty][0]' "$tmp/workflows-show.json")" --json >/dev/null
done
