# The smoke daemon has no [docbank], so attachments are said to be unavailable, plainly: the Attachments tab names the missing
# Docbank, @ opens the path form with that message as its error, U says the Issue has none, and `tern-kata migrate-attachments`
# refuses with the same words. `tern-kata attach` is gone and says what to use. Own Issue; it cleans up.
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

# From the shell: the migration refuses plainly, and the removed commands say what replaced them.
if (cd "$at_dir" && "$tmp/tern-kata" migrate-attachments --as smoke-shell) >"$tmp/migrate.out" 2>"$tmp/migrate.err"; then
  echo "migrate-attachments succeeded against a daemon without Docbank" >&2
  exit 1
fi
grep -q "no Docbank configured" "$tmp/migrate.err"
if (cd "$at_dir" && "$tmp/tern-kata" attach "$at_short" "$tmp/workspace/.kata.toml") >/dev/null 2>"$tmp/attach.err"; then
  echo "tern-kata attach still runs" >&2
  exit 1
fi
grep -q "kata docbank attachments add" "$tmp/attach.err"
