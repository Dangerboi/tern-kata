# New Project: the picker of a bound directory and the sidebar of an unbound one end with a row that opens a one-field form.
# The form starts with the directory's name, so the names below extend it. kata keeps the name of an archived Project
# reserved, which gives the form a rejection to show.
kata projects create workspace-archived --json >/dev/null
kata projects remove workspace-archived --force --json >/dev/null
np_heading="[data-id='main.topbar.left.heading']"
np_form="[data-id='layer.project']"
np_input="[data-id='layer.project.name']"
np_error="[data-id='layer.project.error']"
np_picker_rows="[data-id^='layer.collections.list.']"
np_sidebar_rows="[data-id^='main.columns.collections.']"
np_issue_rows="[data-id^='main.columns.list.issues.01']"
np_launch() { # run kata in directory $1 from an idle shell
  printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$1" "$KATA_HOME" "$tmp" >"$tmp/np-launch.ctl"
  ctl --file "$tmp/np-launch.ctl"
}
np_projects() { kata projects list --json | jq -r '[.projects[].name] | join(",")'; }

# Bound directory: the picker ends with New Project, whose form opens over it with the directory's name.
quit_app
ctl --file "$tmp/launch.ctl"
expect_tree "$np_heading" '.ok and .nodes[0].text == "Project: smoke"'
ctl click "[data-id='main.topbar.right.menu']"
ctl key End
expect_tree "$np_picker_rows" '.ok and .nodes[-1].text == "New Project" and .nodes[-1]["aria-selected"] == "true"'
ctl key enter
expect_tree "$np_form" '.ok and .count == 1'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$np_input" '.ok and .count == 1 and (.nodes[0].text | startswith("workspace"))'
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("enter create Project") and contains("esc cancel"))'

# kata's rejection stays in the form with the name; editing the name clears it.
ctl type '"-archived"'
ctl key enter
expect_tree "$np_error" '.ok and .count == 1 and (.nodes[0].text | contains("archived") and contains("kata projects restore workspace-archived"))'
expect_tree "$np_input" '.ok and (.nodes[0].text | startswith("workspace-archived"))'
ctl key Backspace
expect_tree "$np_error" '.ok and .count == 0'
expect_tree "$np_input" '.ok and (.nodes[0].text | startswith("workspace-archive") and (startswith("workspace-archived") | not))'

# esc leaves the form for the picker, as it was, also after a click inside the form.
ctl click "$np_input"
ctl key esc
expect_tree "$np_form" '.ok and .count == 0'
expect_tree "$np_picker_rows" '.ok and .nodes[-1]["aria-selected"] == "true"'

# A click on the row is enter on it. Here the form only creates: kata switches to the new Project and the directory stays bound.
ctl click "[data-id='layer.collections.list.new-project']"
expect_tree "$np_form" '.ok and .count == 1'
ctl type '"-extra"'
ctl key enter
expect_tree "$np_form" '.ok and .count == 0'
expect_tree "[data-id='layer.collections']" '.ok and .count == 0'
expect_tree "$np_heading" '.ok and .nodes[0].text == "Project: workspace-extra"'
[[ $(np_projects) == *workspace-extra* ]]
grep -qx 'name = "smoke"' "$tmp/workspace/.kata.toml"
ctl click "[data-id='main.topbar.right.menu']"
expect_tree "$np_picker_rows" '.ok and .nodes[0].text == "smoke"'
ctl click "[data-id='layer.collections.list.projects']"
expect_tree "$np_picker_rows" '.ok and ([.nodes[] | select(.text == "workspace-extracurrent")] | length == 1)'
ctl key esc
ctl key Home
ctl key enter
expect_tree "$np_heading" '.ok and .nodes[0].text == "Project: smoke"'

# Unbound directory: the sidebar ends with New Project; the click opens the form and esc closes it, leaving the cursor on the row.
quit_app
mkdir -p "$tmp/np-unbound"
np_launch "$tmp/np-unbound"
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 1'
expect_tree "$np_sidebar_rows" '.ok and .nodes[-1].text == "New Project"'
ctl click "[data-id='main.columns.collections.new-project']"
expect_tree "$np_form" '.ok and .count == 1'
expect_tree "$np_input" '.ok and (.nodes[0].text | startswith("np-unbound"))'
ctl key esc
expect_tree "$np_form" '.ok and .count == 0'
expect_tree "$np_sidebar_rows" '.ok and .nodes[-1]["aria-selected"] == "true"'

# enter on the row, then enter on the directory's name: kata creates the Project, the directory is bound to it, and the sidebar goes.
ctl key enter
ctl key enter
expect_tree "$np_heading" '.ok and .nodes[0].text == "Project: np-unbound"'
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 0'
expect_tree "$np_form" '.ok and .count == 0'
grep -qx 'name = "np-unbound"' "$tmp/np-unbound/.kata.toml"
grep -qx '.kata.local.toml' "$tmp/np-unbound/.gitignore"
kata --workspace "$tmp/np-unbound" create "Created after New Project" --json >/dev/null
expect_tree "$np_issue_rows" '.ok and .count == 1 and (.nodes[0].text | contains("Created after New Project"))'

# The binding is what kata finds next time: it opens project-first.
quit_app
np_launch "$tmp/np-unbound"
expect_tree "$np_heading" '.ok and .nodes[0].text == "Project: np-unbound"'
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 0'
expect_tree "$np_issue_rows" '.ok and .count == 1'

# A .kata.toml naming a Project this kata has never seen is an error, not an Inbox: the Pane shows kata's not-registered message and the `kata init` hint, and registers nothing.
quit_app
mkdir -p "$tmp/np-stale"
printf 'version = 1\n\n[project]\nname = "np-stale-bound"\n' >"$tmp/np-stale/.kata.toml"
np_launch "$tmp/np-stale"
expect_tree "[data-id='main.error']" '([.nodes[] | .. | objects | .text? // empty] | join(" ")) | contains("project np-stale-bound is not registered") and contains("run \"kata init\" in this workspace")'
expect_tree "[data-id='main.columns.collections']" '.ok and .count == 0'
[[ $(np_projects) != *np-stale* ]]

# Restore the bound Project and remove this step's fixtures.
quit_app
for project in workspace-extra np-unbound; do
  kata projects remove "$project" --force --json >/dev/null
done
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
