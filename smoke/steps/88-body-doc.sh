# On a Tern that draws a doc (0.9.26 or newer) the Issue body is one: a double click edits it in place and leaving it saves the
# edit at once, and a selection takes a Comment that lands in the Comments tab as a kata comment quoting the selection. Own
# Project and Issue; it cleans up.
if ! tern_at_least 0.9.26; then
  echo "skipped: this Tern does not draw a doc"
else
  bd_dir="$tmp/bodydoc-workspace"
  mkdir -p "$bd_dir"
  kata --workspace "$bd_dir" init --project bodydoc --json >/dev/null
  kata --workspace "$bd_dir" create "Doc body Issue" --body $'Intro line\n\nSecond paragraph' --json >"$tmp/bd-issue.json"
  bd_uid=$(jq -r '.issue.uid' "$tmp/bd-issue.json")
  bd_short=$(jq -r '.issue.short_id' "$tmp/bd-issue.json")
  bd_body="[data-id^='main.columns.detail.viewport.content.bodydoc-']"
  bd_stored_body() { kata --workspace "$bd_dir" show "$bd_uid" --json | jq -r '.issue.body'; }
  bd_body_is() { [[ $(bd_stored_body) == "$1" ]]; }

  quit_app
  printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$bd_dir" "$KATA_HOME" "$tmp" >"$tmp/bd-launch.ctl"
  ctl --file "$tmp/bd-launch.ctl"
  expect_tree "[data-id\$='.$bd_uid']" '.ok and .count == 1'
  ctl click "[data-id\$='.$bd_uid']"
  expect_tree "$bd_body" '.ok and .count == 1 and (.nodes[0].class | contains("sf-doc")) and (.nodes[0].text | contains("Second paragraph"))'

  # A selection shows the Comment bubble; the note goes to kata as a comment that quotes the selection.
  read -r bd_x bd_y < <(ctl tree "$bd_body p" | jq -r '.nodes[0].rect | "\(.[0] + 2) \(.[1] + 9)"')
  ctl down "$bd_x" "$bd_y"
  ctl move "$((${bd_x%.*} + 40))" "${bd_y%.*}"
  ctl move "$((${bd_x%.*} + 90))" "${bd_y%.*}"
  ctl up "$((${bd_x%.*} + 90))" "${bd_y%.*}"
  ctl wait 1000 # the bubble shows once the selection has been still for a moment of app time
  expect_tree "$bd_body .sf-doc-bubble-comment" '.ok and .count == 1'
  ctl click "$bd_body .sf-doc-bubble-comment"
  expect_tree "$bd_body .sf-doc-field" '.ok and .count == 1'
  ctl click "$bd_body .sf-doc-field"
  ctl type '"smoke selection note"'
  ctl key Enter
  bd_comment_quotes() { kata --workspace "$bd_dir" show "$bd_short" --json | jq -e '.comments | length == 1 and (.[0].body | startswith("> ") and endswith("\n\nsmoke selection note"))' >/dev/null; }
  wait_for bd_comment_quotes
  ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']"
  expect_tree "[data-id^='main.columns.detail.viewport.content.activity.']" '[.nodes[] | .text // empty] | any(contains("smoke selection note"))'
  expect_tree "[data-id='layer.editor']" '.ok and .count == 0'

  # A double click edits in place and the keys go to the doc; clicking the title leaves it, which saves the edit without the
  # Edit Issue body overlay (Tern sends the text and then that the edit ended).
  ctl dblclick "$bd_body"
  expect_tree "$bd_body" '.ok and (.nodes[0].class | contains("editing"))'
  ctl key z
  ctl key y
  ctl click "[data-id='main.columns.detail.viewport.content.heading.title']"
  wait_for bd_body_is $'Intro line\n\nSecond paragraphzy'
  expect_tree "$bd_body" '.ok and (.nodes[0].class | contains("editing") | not) and (.nodes[0].text | contains("Second paragraphzy"))'
  expect_tree "[data-id='layer.editor']" '.ok and .count == 0'

  # A modal drawn over a doc is above it. A body long enough to run behind the Edit Issue body card is a doc under the card's
  # Cancel button: a click there asks to discard the Draft. (On a doc Tern lifts above the layer the click lands on the doc and
  # nothing happens, unless the plugin's stylesheet isolates the doc.) Its own Issue, which the Project's removal below takes.
  kata --workspace "$bd_dir" create "Doc long body Issue" --body "$(printf 'Long body line %s\n\n' $(seq 1 24))" --json >"$tmp/bd-long.json"
  bd_long_uid=$(jq -r '.issue.uid' "$tmp/bd-long.json")
  expect_tree "[data-id\$='.$bd_long_uid']" '.ok and .count == 1'
  ctl click "[data-id\$='.$bd_long_uid']"
  expect_tree "$bd_body" '.ok and .count == 1 and (.nodes[0].class | contains("sf-doc")) and (.nodes[0].text | contains("Long body line 12"))'
  # The margin card of the selection comment made on the first Issue stays with that Issue's doc: this one's margin is empty.
  expect_tree "$bd_body .sf-doc-card" '.ok and .count == 0'
  ctl key e
  expect_tree "[data-id='layer.editor.input']" '.ok and .count == 1'
  ctl type '"edit"'
  ctl click "[data-id^='layer.editor.'][data-id\$='.cancel']"
  expect_tree "[data-id^='layer.editor.'][data-id\$='.keep']" '.ok and .count == 1'
  ctl key y
  expect_tree "[data-id='layer.editor']" '.ok and .count == 0'

  quit_app
  kata projects remove bodydoc --force --json >/dev/null
  ctl --file "$tmp/launch.ctl"
  ctl plugins expect smoke
  expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
fi
