# M on the selected Issue opens the Move card: the list of other Projects takes a typed filter, enter previews the move with
# kata's dry run (it names the Project, not the new id) and enter again moves. A Notice then names the new project#id as a link,
# and clicking it opens the Issue in its new Project. Own Issue and Project; it deletes the Issue. Move an Inbox Issue the same
# way by designating the Inbox Project first: that takes a raw project-metadata request, which the kata CLI does not send, so
# this step moves a Project Issue and leaves the Inbox Collection to the driver tests.
mv_dir="$tmp/workspace"
kata projects create move-target --json >"$tmp/move-target.json"
kata --workspace "$mv_dir" create "Smoke move Issue" --body "Moves to another Project" --json >"$tmp/move-issue.json"
mv_uid=$(jq -r '.issue.uid' "$tmp/move-issue.json")
mv_row="[data-id='main.columns.list.issues.$mv_uid']"
expect_tree "$mv_row" '.ok and .count == 1'
ctl click "$mv_row"
expect_tree "$mv_row" '.ok and .nodes[0]["aria-selected"] == "true"'

ctl key M
expect_tree "[data-id='layer.move']" '.ok and .count == 1'
# The list holds the other Projects and not this one.
expect_tree "[data-id^='layer.move.list.']" '.ok and ([.nodes[].text] | any(contains("move-target"))) and ([.nodes[].text] | all(contains("smoke") | not))'
ctl type '"move-t"'
expect_tree "[data-id^='layer.move.list.']" '.ok and ([.nodes[] | select(.text != null and (.text | contains("move-target")))] | length) >= 1'
ctl key enter
expect_tree "[data-id='layer.move.preview']" '.ok and .count == 1 and (.nodes[0].text | contains("moves to move-target"))'
# The Issue has not moved: the preview changed nothing.
expect_tree "$mv_row" '.ok and .count == 1'

ctl key enter
expect_tree "[data-id='layer.move']" '.ok and .count == 0'
expect_tree "[data-id\$='.notice']" '.ok and .count == 1 and (.nodes[0].text | contains("move-target#"))'
expect_tree "$mv_row" '.ok and .count == 0'

# The notice is a link: a click opens the Issue in its new Project.
mv_ref=$(ctl tree "[data-id\$='.notice']" | jq -er '.nodes[0].text | capture("(?<ref>move-target#[a-z0-9]+)").ref')
ctl click "[data-id\$='.notice']"
expect_tree "[data-id='main.topbar.left.heading']" '.ok and (.nodes[0].text | contains("Project: move-target"))'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke move Issue")))'

kata delete "$mv_ref" --force --confirm "DELETE $mv_ref" --json >/dev/null
quit_app
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
