# The smoke daemon has no [docbank], so attachments are said to be unavailable, plainly: the Attachments tab names the missing
# Docbank, @ opens the path form with that message as its error, U says the Issue has none, and `tern-kata migrate-attachments`
# refuses with the same words. Own Issue; it cleans up.
at_dir="$tmp/workspace"
kata --workspace "$at_dir" create "Smoke attachments Issue" --json >"$tmp/attachments.json"
at_uid=$(jq -r '.issue.uid' "$tmp/attachments.json")
at_short=$(jq -r '.issue.short_id' "$tmp/attachments.json")
at_tab="[data-id='main.columns.detail.viewport.content.activity.tabs.tab3']"
expect_tree "[data-id='main.columns.list.issues.$at_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$at_uid']"
expect_tree "[data-id='main.columns.list.issues.$at_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl click "$at_tab"
expect_tree "[data-id='main.columns.detail.viewport.content.activity.empty']" '.ok and .count == 1 and (.nodes[0].text | startswith("This kata daemon has no Docbank configured"))'
expect_tree "$at_tab" '.ok and (.nodes[0].text | contains("Attachments (0)"))'

# @ opens the path form with the reason as its error; Escape closes it and nothing was sent.
ctl key '@'
expect_tree "[data-id='layer.fields.input']" '.ok and .count == 1'
expect_tree "[data-id='layer.fields.error']" '.ok and .count == 1 and (.nodes[0].text | contains("no Docbank configured"))'
ctl key Escape
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'

# U on an Issue with no attachments says so without opening a form.
ctl key U
expect_tree "[data-id='main.notice']" '.ok and .count == 1 and (.nodes[0].text | contains("no attachments"))'
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'

# The Close card of an Issue with no attachments has no attachment picker (the Message field is there, so the card is drawn).
ctl key x
expect_tree "[data-id='layer.close.message']" '.ok and .count == 1'
expect_tree "[data-id='layer.close.attachments']" '.ok and .count == 0'
ctl key Escape
expect_tree "[data-id='layer.close']" '.ok and .count == 0'

# From the shell: the migration refuses plainly.
if (cd "$at_dir" && "$tmp/tern-kata" migrate-attachments --as smoke-shell) >"$tmp/migrate.out" 2>"$tmp/migrate.err"; then
  echo "migrate-attachments succeeded against a daemon without Docbank" >&2
  exit 1
fi
grep -q "no Docbank configured" "$tmp/migrate.err"

kata --workspace "$at_dir" delete "$at_uid" --force --confirm "DELETE smoke#$at_short" --json >/dev/null
expect_tree "[data-id='main.columns.list.issues.$at_uid']" '.ok and .count == 0'
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']"
