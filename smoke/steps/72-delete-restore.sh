# X on the selected Issue opens the Delete card: it refuses until the qualified id is typed, then deletes the Issue with kata's
# confirmation and a reason; u right after restores it; deleting it again and choosing "Restore a deleted Issue" in the Menu
# brings it back by project#id. Own Issue and Project; it leaves the Issue restored and then deletes it.
dl_dir="$tmp/workspace"
kata --workspace "$dl_dir" create "Smoke delete Issue" --json >"$tmp/delete-issue.json"
dl_uid=$(jq -r '.issue.uid' "$tmp/delete-issue.json")
dl_short=$(jq -r '.issue.short_id' "$tmp/delete-issue.json")
dl_ref="smoke#$dl_short"
dl_row="[data-id='main.columns.list.issues.$dl_uid']"
dl_notice="[data-id='main.notice']"
expect_tree "$dl_row" '.ok and .count == 1'
ctl click "$dl_row"
expect_tree "$dl_row" '.ok and .nodes[0]["aria-selected"] == "true"'

# The card names the Issue and says it can be restored. A wrong id is refused and nothing is deleted.
ctl key X
expect_tree "[data-id='layer.delete.issue']" ".ok and .count == 1 and (.nodes[0].text | startswith(\"$dl_ref \"))"
expect_tree "[data-id='layer.delete.note']" '.ok and .count == 1 and (.nodes[0].text | contains("can be restored"))'
ctl type '"smoke#wrong"'
ctl key enter
expect_tree "[data-id='layer.delete.error']" ".ok and .count == 1 and (.nodes[0].text | contains(\"$dl_ref\"))"
kata --workspace "$dl_dir" show "$dl_uid" --json | jq -e '.issue.deleted_at == null' >/dev/null
expect_tree "$dl_row" '.ok and .count == 1'

# The exact id, and a reason in the second field, delete it.
ctl key ctrl+u
ctl type "\"$dl_ref\""
ctl key tab
ctl type '"smoke duplicate"'
ctl key enter
expect_tree "[data-id='layer.delete']" '.ok and .count == 0'
expect_tree "$dl_notice" ".ok and .count == 1 and (.nodes[0].text == \"Deleted $dl_ref. Press u to restore it.\")"
expect_tree "$dl_row" '.ok and .count == 0'
if kata --workspace "$dl_dir" show "$dl_uid" --json >/dev/null 2>&1; then
  echo "the Issue is still shown after the delete" >&2
  exit 1
fi

# u right after restores it.
ctl key u
expect_tree "$dl_notice" ".ok and .count == 1 and (.nodes[0].text | startswith(\"undid delete on \"))"
expect_tree "$dl_row" '.ok and .count == 1'
kata --workspace "$dl_dir" show "$dl_uid" --json | jq -e '.issue.deleted_at == null' >/dev/null

# Delete it again, then restore it from the Menu by project#id.
ctl click "$dl_row"
ctl key X
ctl type "\"$dl_ref\""
ctl key enter
expect_tree "[data-id='layer.delete']" '.ok and .count == 0'
expect_tree "$dl_row" '.ok and .count == 0'

ctl key escape
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.list.restore']"
expect_tree "[data-id='layer.fields']" '.ok and .count == 1'
ctl type "\"$dl_ref\""
ctl key enter
expect_tree "[data-id='layer.fields.found']" '.ok and .count == 1 and (.nodes[0].text | contains("Smoke delete Issue"))'
expect_tree "[data-id='layer.fields.found-state']" '.ok and .count == 1 and (.nodes[0].text | contains("is deleted"))'
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
expect_tree "$dl_notice" ".ok and .count == 1 and (.nodes[0].text == \"Restored $dl_ref.\")"
expect_tree "$dl_row" '.ok and .count == 1'
kata --workspace "$dl_dir" show "$dl_uid" --json | jq -e '.issue.deleted_at == null' >/dev/null

kata --workspace "$dl_dir" delete "$dl_ref" --force --confirm "DELETE $dl_ref" --json >/dev/null
expect_tree "$dl_row" '.ok and .count == 0'
