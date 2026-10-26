# The Workflows Page (5) lists the Project's workflows. Enter on a selected one shows its steps beside the list, and a double
# click on a workflow that is not selected does the same for that workflow. Own workflows; it cleans up.
wf_dir="$tmp/workspace"
wf_make() { # uid name command
  jq -n --arg name "$2" --arg command "$3" '{name: $name, definition: {version: 1, steps: [{key: "only", kind: "command", command: $command}]}}' >"$tmp/workflows-$1.json"
  kata --workspace "$wf_dir" cron workflow create --uid "$1" --file "$tmp/workflows-$1.json" --json >/dev/null
}
wf_first=01H5MK0000000000000000W001
wf_second=01H5MK0000000000000000W002
wf_make "$wf_first" "Smoke first workflow" "echo first-step"
wf_make "$wf_second" "Smoke second workflow" "echo second-step"

ctl key 5
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
for wf_uid in "$wf_first" "$wf_second"; do
  kata --workspace "$wf_dir" cron workflow show "$wf_uid" --json >"$tmp/workflows-show.json"
  kata --workspace "$wf_dir" cron workflow delete "$wf_uid" --expected-event-uid "$(jq -r '[.. | .definition_event_uid? // empty][0]' "$tmp/workflows-show.json")" --json >/dev/null
done
