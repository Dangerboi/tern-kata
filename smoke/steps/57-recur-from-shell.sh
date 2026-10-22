# tern-kata recur from a shell makes an Issue recur weekly, and the open Pane's Issue detail reads the schedule. Closing it done
# with the kata CLI creates the next Occurrence; recur changes the schedule, prints it, and stops it, and the detail agrees at
# each step. Own Issue; it cleans up.
rs_dir="$tmp/workspace"
kata --workspace "$rs_dir" create "Smoke shell recurrence" --body "Recurs from a shell" --json >"$tmp/shell-recurring.json"
rs_uid=$(jq -r '.issue.uid' "$tmp/shell-recurring.json")
rs_short=$(jq -r '.issue.short_id' "$tmp/shell-recurring.json")
expect_tree "[data-id='main.columns.list.issues.$rs_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$rs_uid']"
expect_tree "[data-id='main.columns.list.issues.$rs_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'

rs_recur() { (cd "$rs_dir" && "$tmp/tern-kata" recur "$rs_short" "$@" --as smoke-shell); }
[[ $(rs_recur) == *"does not recur"* ]]
rs_out=$(rs_recur --frequency weekly --on mon,thu --start 2026-11-02 --zone UTC)
[[ $rs_out == *"recurs: Weekly on Mon, Thu"* ]]
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Weekly on Mon, Thu")))'
[[ $(rs_recur) == "$rs_out" ]]

kata --workspace "$rs_dir" close "$rs_short" --done --message "Finished the shell recurring fixture and verified it by hand" --commit abc1234def56 --json >/dev/null
expect_tree "[data-id^='main.columns.list.issues.']" '.ok and ([.nodes[] | select((.text | contains("open ↻ Smoke shell recurrence")) and (.text | startswith("'"$rs_short"' ") | not))] | length == 1)'

# The first Occurrence is closed; the detail of the open Pane is still its own, and the series is read through it.
rs_out=$(rs_recur --frequency weekly --every 2 --on fri --start 2026-11-02 --zone UTC)
[[ $rs_out == *"recurs: Every 2 weeks on Fri"* ]]
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Every 2 weeks on Fri")))'
[[ $(rs_recur --stop) == *"no longer recurs"* ]]
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("stopped")))'
[[ $(rs_recur) == *"was stopped"* ]]
if rs_recur --stop >/dev/null 2>"$tmp/shell-recurring.err"; then false; fi
grep -q "was stopped; nothing was written" "$tmp/shell-recurring.err"

kata --workspace "$rs_dir" list --status all --json >"$tmp/shell-recurring-list.json"
for uid in $(jq -r '.issues[] | select(.title == "Smoke shell recurrence") | .uid' "$tmp/shell-recurring-list.json"); do
  short=$(jq -r --arg uid "$uid" '.issues[] | select(.uid == $uid) | .short_id' "$tmp/shell-recurring-list.json")
  kata --workspace "$rs_dir" delete "$uid" --force --confirm "DELETE smoke#$short" --json >/dev/null
done
