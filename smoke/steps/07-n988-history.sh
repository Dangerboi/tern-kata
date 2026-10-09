# Change history reads as one sentence per event, never as JSON. The events all
# exist before the Pane starts, so no live refresh reflows the list under the click.
quit_app
kata --as dana --workspace "$tmp/workspace" create "Smoke history Issue" --body "History body" --json >"$tmp/history.json"
history_uid=$(jq -r '.issue.uid' "$tmp/history.json")
history_short=$(jq -r '.issue.short_id' "$tmp/history.json")
kata --as dana --workspace "$tmp/workspace" assign "$history_short" alice --json
kata --as dana --workspace "$tmp/workspace" edit "$history_short" --priority 1 --json
kata --as dana --workspace "$tmp/workspace" label add "$history_short" history --json
kata --as dana --workspace "$tmp/workspace" close "$history_short" --done --message "Finished the history fixture and verified it by hand" --commit abc1234def56 --json
kata --as dana --workspace "$tmp/workspace" reopen "$history_short" --json
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id='main.columns.list.issues.$history_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$history_uid']"
expect_tree "[data-id='main.columns.detail.heading.title']" '.nodes[0].text | contains("Smoke history Issue") and (contains("loading") | not)'
history_rows="[data-id^='main.columns.detail.sections.section5.']"
history_text() { printf '[.nodes[] | .text? // empty] | join(" | ") | %s' "$1"; }
expect_tree "$history_rows" "$(history_text 'contains("dana assigned to alice") and contains("dana closed: done") and contains("dana reopened") and contains("dana set priority to 1") and contains("dana added label `history`")')"
expect_tree "$history_rows" "$(history_text 'contains("Finished the history fixture") and contains("commit abc1234") and (contains("```") | not) and (contains("{\"") | not) and (contains("old_priority") | not)')"

quit_app
kata --workspace "$tmp/workspace" delete "$history_uid" --force --confirm "DELETE smoke#$history_short" --json
ctl --file "$tmp/launch.ctl"
