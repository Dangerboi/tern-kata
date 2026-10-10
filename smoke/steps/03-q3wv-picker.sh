# Fixture: a second Project to switch to. The bound Project sorts before it, so pinning is visible only against Inbox.
kata projects create picker-alpha --json >"$tmp/picker-alpha.json"
picker_alpha_uid=$(jq -er '.project.uid' "$tmp/picker-alpha.json")
kata --workspace "$tmp/workspace" create "Picker alpha Issue" --project picker-alpha --json >"$tmp/picker-alpha-issue.json"
picker_items="[data-id^='layer.collections.list.']"
picker_heading="[data-id='main.topbar.left.heading']"

# A bound directory opens on its Project with no Menu sidebar.
expect_tree "$picker_heading" '.ok and .count == 1 and .nodes[0].text == "Project: smoke"'
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

# p opens the Menu at its top level: the bound Project first and marked current, the fixed Collections, then Projects and New Project. No other Project is listed inline.
picker_projects_entry="[data-id='layer.collections.list.projects']"
ctl key p
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
expect_tree "$picker_items" '.ok and .count == 9 and .nodes[0].text == "smokecurrent" and .nodes[0]["aria-selected"] == "true" and .nodes[1].text == "Inbox" and .nodes[-1].text == "New Project"'
expect_tree "$picker_items" '.nodes[-2].text == "Projects"'
expect_tree "[data-id^='layer.collections.list.project.']" '.ok and .count == 1'
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("enter open Collection") and contains("esc close"))'
ctl key End
expect_tree "$picker_items" '.nodes[-1]["aria-selected"] == "true"'
ctl key Up
expect_tree "$picker_items" '.nodes[-2]["aria-selected"] == "true"'
ctl key Home
expect_tree "$picker_items" '.nodes[0]["aria-selected"] == "true"'
ctl key Down
expect_tree "$picker_items" '.nodes[1]["aria-selected"] == "true"'
ctl key Up
expect_tree "$picker_items" '.nodes[0]["aria-selected"] == "true"'

# Typing filters; esc closes the picker without switching.
for letter in a l p h a; do ctl key "$letter"; done
expect_tree "$picker_items" '.ok and .count == 1 and .nodes[0].text == "picker-alpha"'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Project: smoke"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count == 3'

# enter switches to the filtered Collection; reopening shows it as current while the bound Project stays first.
ctl key p
for letter in a l p h a; do ctl key "$letter"; done
ctl key enter
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Project: picker-alpha"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count == 1 and (.nodes[0].text | contains("Picker alpha Issue"))'
ctl key p
expect_tree "$picker_items" '.ok and .nodes[0].text == "smoke" and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id='layer.collections.list.project.$picker_alpha_uid']" '.ok and .count == 0'
# enter on Projects lists every Project with the current one marked and the cursor on it; esc returns to the top level, still open.
ctl key End
ctl key Up
expect_tree "$picker_projects_entry" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key enter
expect_tree "$picker_items" '.ok and .nodes[0].text == "Back" and (.nodes | map(.text) | index("picker-alphacurrent") != null)'
expect_tree "[data-id='layer.collections.list.project.$picker_alpha_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("esc back"))'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
expect_tree "$picker_projects_entry" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key Home
ctl key enter
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Project: smoke"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count == 3'

# A click on a row switches like enter; the top bar title opens the picker; esc closes it, also after a click inside it.
ctl key p
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "$picker_projects_entry"
expect_tree "[data-id='layer.collections.list.project.$picker_alpha_uid']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.list.project.$picker_alpha_uid']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Project: picker-alpha"'
ctl click "$picker_heading"
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl click "[data-id='layer.collections.filter']"
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Project: picker-alpha"'
ctl key p
ctl key enter
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Project: smoke"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count == 3'

# esc backs out detail, then the list, into the picker, then closes it.
ctl key enter
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("esc back"))'
ctl key esc
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("esc Menu"))'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'

# A narrow Pane still folds to the focused column, and the picker opens over it.
read -r picker_width picker_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')
ctl resize 800 "$picker_height"
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 0'
ctl key p
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl key esc
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
ctl resize "$picker_width" "$picker_height"
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'

# An unbound directory keeps the sidebar, starting on Inbox, and esc from the list still focuses it.
quit_app
mkdir -p "$tmp/picker-unbound"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/picker-unbound" "$KATA_HOME" "$tmp" >"$tmp/picker-unbound.ctl"
ctl --file "$tmp/picker-unbound.ctl"
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.collections.inbox']" '.ok and .nodes[0]["aria-selected"] == "true"'
expect_tree "$picker_heading" '.ok and .nodes[0].text == "Inbox"'
ctl key esc
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("next Collection"))'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
quit_app

# Restore the bound Project and remove this step's fixture.
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
kata projects remove picker-alpha --force --json
