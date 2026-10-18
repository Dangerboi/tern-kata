# Tern delivers ctrl+n, ctrl+p, ctrl+v, alt+v and I to the Pane: Emacs keys move and page the list, the Menu
# sidebar and the search results, and I opens the Inbox from the list, the Issue detail and the Menu sidebar.
ek_dir="$tmp/emacs-keys-workspace"
mkdir -p "$ek_dir" "$tmp/emacs-keys-unbound"
kata --workspace "$ek_dir" init --project emacs-keys --json >/dev/null
for title in first second third; do kata --workspace "$ek_dir" create "Emacs keys $title" --json >/dev/null; done

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$ek_dir" "$KATA_HOME" "$tmp" >"$tmp/ek-launch.ctl"
ctl --file "$tmp/ek-launch.ctl"
ek_rows="[data-id^='main.columns.list.issues.']"
expect_tree "$ek_rows" '.ok and .count == 3'
ctl key Home
expect_tree "$ek_rows" '.nodes[0]["aria-selected"] == "true"'
ctl key Ctrl+n
expect_tree "$ek_rows" '.nodes[1]["aria-selected"] == "true"'
ctl key Ctrl+p
expect_tree "$ek_rows" '.nodes[0]["aria-selected"] == "true"'
ctl key Ctrl+v
expect_tree "$ek_rows" '.nodes[-1]["aria-selected"] == "true"'
ctl key Alt+v
expect_tree "$ek_rows" '.nodes[0]["aria-selected"] == "true"'

# Search keeps typing for text: I is a letter there, and the Emacs keys move the results.
ctl key /
ctl key I
ctl key Ctrl+n
expect_tree "[data-id='main.topbar.left.heading']" '.ok and .nodes[0].text == "Project: emacs-keys"'
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("commit search"))'
ctl key Escape
ctl key Escape
expect_tree "[data-id='dock.hints']" '.ok and (.nodes[0].text | contains("↑↓ move"))'

# The Issue detail: I opens the Inbox from it.
ctl key Tab
ctl key I
expect_tree "[data-id='main.topbar.left.heading']" '.ok and .nodes[0].text == "Inbox"'

# The Menu sidebar (an unbound directory shows it): the Emacs keys move it, and I opens the Inbox.
quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/emacs-keys-unbound" "$KATA_HOME" "$tmp" >"$tmp/ek-unbound.ctl"
ctl --file "$tmp/ek-unbound.ctl"
ek_entries="[data-id^='main.columns.collections.']"
expect_tree "$ek_entries" '.ok and .count >= 7'
ctl key Escape
ctl key Home
expect_tree "$ek_entries" '.nodes[0]["aria-selected"] == "true"'
ctl key Ctrl+n
expect_tree "$ek_entries" '.nodes[1]["aria-selected"] == "true"'
ctl key Ctrl+p
expect_tree "$ek_entries" '.nodes[0]["aria-selected"] == "true"'
ctl key End
ctl key I
expect_tree "[data-id='main.topbar.left.heading']" '.ok and .nodes[0].text == "Inbox"'

quit_app
ctl --file "$tmp/launch.ctl"
