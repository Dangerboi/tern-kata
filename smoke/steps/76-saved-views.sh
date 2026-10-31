# A saved view is made in the Menu's Views level with the View card, survives a restart, opens with its base and filters and is named
# on the filter line, and is deleted after a y/n question. The step ends with no view in views.toml and no filters on.
sv_file="$XDG_CONFIG_HOME/tern-kata/views.toml"
sv_items="[data-id^='layer.collections.list.']"
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

# New view is the last row of the Views level; the card opens with the current Collection as its base.
ctl click "[data-id='main.topbar.right.menu']"
ctl key Down
ctl key enter
expect_tree "$sv_items" '.ok and .nodes[0].text == "Back" and .nodes[-1].text == "New view"'
ctl key End
ctl key enter
expect_tree "[data-id='layer.view']" '.ok and .count == 1'
for letter in o p e n b u g s; do ctl key "$letter"; done
ctl key Tab
ctl key Tab
ctl key Tab
ctl key Right
expect_tree "[data-id='layer.view.status']" '.ok and (.nodes[0].text | contains("[ open ]"))'

# A refusal names the field and keeps the card with its input: a fixed Collection's name is not free.
for _ in 1 2 3 4 5; do ctl key Tab; done
expect_tree "[data-id='layer.view.name']" '.ok and .count == 1'
for _ in 1 2 3 4 5 6 7 8; do ctl key backspace; done
for letter in t o d a y; do ctl key "$letter"; done
ctl key Ctrl+o
expect_tree "[data-id='layer.view']" '.ok and .count == 1'
expect_tree "[data-id='layer.view.error']" '.ok and (.nodes[0].text | startswith("Name"))'
if [[ -f "$sv_file" ]] && grep -q '\[\[view\]\]' "$sv_file"; then printf 'views.toml holds a view after the refused save\n' >&2; return 1; fi
for _ in 1 2 3 4 5; do ctl key backspace; done
for letter in o p e n b u g s; do ctl key "$letter"; done
ctl key Ctrl+o
expect_tree "[data-id='layer.view']" '.ok and .count == 0'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "[data-id='main.filters']" '.ok and (.nodes[0].text | contains("status=open") and endswith("(openbugs)"))'
grep -q 'name = "openbugs"' "$sv_file"
grep -q 'status = "open"' "$sv_file"

# A new Pane reads the file: no filters until the view is opened from the Menu.
quit_app
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id='main.filters']" '.ok and .count == 0'
ctl click "[data-id='main.topbar.right.menu']"
for letter in o p e n b u g s; do ctl key "$letter"; done
expect_tree "$sv_items" '.ok and .count == 1 and .nodes[0].text == "openbugs"'
ctl key enter
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "[data-id='main.filters']" '.ok and (.nodes[0].text | contains("status=open") and endswith("(openbugs)"))'

# c clears the filters for this Pane only; the view stays in the file.
ctl key c
expect_tree "[data-id='main.filters']" '.ok and .count == 0'
grep -q 'name = "openbugs"' "$sv_file"

# Delete asks first; n keeps the view, y removes it.
ctl click "[data-id='main.topbar.right.menu']"
for letter in o p e n b u g s; do ctl key "$letter"; done
ctl key Ctrl+d
expect_tree "[data-id='layer.collections.delete-view']" '.ok and (.nodes[0].text == "Delete view openbugs? y/n")'
ctl key n
expect_tree "[data-id='layer.collections.delete-view']" '.ok and .count == 0'
grep -q 'name = "openbugs"' "$sv_file"
ctl key Ctrl+d
ctl key y
expect_tree "$sv_items" '.ok and .count == 0'
if grep -q '\[\[view\]\]' "$sv_file"; then printf 'views.toml still holds a view after the delete\n' >&2; return 1; fi
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

# A view added from the shell is listed the next time the Menu opens, shows as JSON groups, and is gone after views rm.
(cd "$tmp/workspace" && "$tmp/tern-kata" views add shellview --base today --owner @me)
grep -q 'name = "shellview"' "$sv_file"
ctl click "[data-id='main.topbar.right.menu']"
for letter in s h e l l v i e w; do ctl key "$letter"; done
expect_tree "$sv_items" '.ok and .count == 1 and .nodes[0].text == "shellview"'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
(cd "$tmp/workspace" && "$tmp/tern-kata" views show shellview --json) | jq -e 'type == "array"' >/dev/null
(cd "$tmp/workspace" && "$tmp/tern-kata" views rm shellview)
if grep -q '\[\[view\]\]' "$sv_file"; then printf 'views.toml still holds a view after views rm\n' >&2; return 1; fi
ctl click "[data-id='main.topbar.right.menu']"
for letter in s h e l l v i e w; do ctl key "$letter"; done
expect_tree "$sv_items" '.ok and .count == 0'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
