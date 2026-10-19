# u on the selected Issue asks for a path and attaches the file; the Attachments tab lists it. `tern-kata attach` run from
# the shell shows in the open Pane without a reload. U picks an Attachment by name and removes it, and the copy leaves the
# Attachment store; the notice says what happened to the file. Own Issue and files; it cleans up.
at_dir="$tmp/workspace"
mkdir -p "$tmp/attach-src"
printf 'shot\n' >"$tmp/attach-src/shot.png"
printf 'live\n' >"$tmp/attach-src/live.log"
kata --workspace "$at_dir" create "Smoke attachments Issue" --json >"$tmp/attachments.json"
at_uid=$(jq -r '.issue.uid' "$tmp/attachments.json")
at_short=$(jq -r '.issue.short_id' "$tmp/attachments.json")
at_tab="[data-id='main.columns.detail.viewport.content.activity.tabs.tab3']"
at_text='[.. | objects | .text? // empty] | join(" ")'
expect_tree "[data-id='main.columns.list.issues.$at_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$at_uid']"
expect_tree "[data-id='main.columns.list.issues.$at_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl click "$at_tab"
expect_tree "[data-id='main.columns.detail.viewport.content.activity']" ".ok and (.nodes[0] | $at_text | contains(\"No attachments, u to attach\"))"

# u opens the path form; an unknown file shows kata's reason in the form, the file is attached once the path is right.
ctl key u
expect_tree "[data-id='layer.fields.value']" '.ok and .count == 1'
ctl type '"/no/such/file.png"'
ctl key enter
expect_tree "[data-id='layer.fields.error']" '.ok and .count == 1 and (.nodes[0].text | contains("does not exist"))'
ctl key Escape
ctl key u
ctl type "$(jq -rn --arg p "\"$tmp/attach-src/shot.png\"" '$p | @json')"
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
expect_tree "[data-id='main.notice']" ".ok and .count == 1 and (.nodes[0].text == \"Attached shot.png to $at_short\")"
expect_tree "$at_tab" '.ok and (.nodes[0].text | contains("Attachments (1)"))'
expect_tree "[data-id='main.columns.detail.viewport.content.activity']" ".ok and (.nodes[0] | $at_text | contains(\"shot.png\"))"
kata --workspace "$at_dir" show "$at_uid" --json >"$tmp/attachments-shown.json"
at_shot=$(jq -r '.issue.metadata["attachment.shot.png"].path' "$tmp/attachments-shown.json")
[[ $at_shot == "$at_dir/.kata-attachments/$at_uid/shot.png" && -f $at_shot ]]

# The command line adds one while the Pane is open.
(cd "$at_dir" && "$tmp/tern-kata" attach "$at_short" "$tmp/attach-src/live.log" --as smoke-shell) >"$tmp/attached-path.txt"
[[ $(<"$tmp/attached-path.txt") == "$at_dir/.kata-attachments/$at_uid/live.log" ]]
expect_tree "$at_tab" '.ok and (.nodes[0].text | contains("Attachments (2)"))'
expect_tree "[data-id='main.columns.detail.viewport.content.activity']" ".ok and (.nodes[0] | $at_text | contains(\"live.log\"))"

# U lists the names; enter removes the highlighted one (live.log sorts first), its file goes and the notice says so.
ctl key U
expect_tree "[data-id='layer.fields.label0']" '.ok and .count == 1 and (.nodes[0].text == "live.log")'
expect_tree "[data-id='layer.fields.label1']" '.ok and .count == 1 and (.nodes[0].text == "shot.png")'
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
expect_tree "[data-id='main.notice']" '.ok and .count == 1 and (.nodes[0].text | startswith("Removed live.log and deleted "))'
expect_tree "$at_tab" '.ok and (.nodes[0].text | contains("Attachments (1)"))'
[[ ! -e "$at_dir/.kata-attachments/$at_uid/live.log" && -f $at_shot ]]

# The last one goes the same way; with none left U says so.
ctl key U
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
[[ ! -e $at_shot ]]
expect_tree "$at_tab" '.ok and (.nodes[0].text | contains("Attachments (0)"))'
ctl key U
expect_tree "[data-id='main.notice']" '.ok and .count == 1 and (.nodes[0].text | contains("no attachments"))'
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'

kata --workspace "$at_dir" delete "$at_uid" --force --confirm "DELETE smoke#$at_short" --json >/dev/null
expect_tree "[data-id='main.columns.list.issues.$at_uid']" '.ok and .count == 0'
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']"
