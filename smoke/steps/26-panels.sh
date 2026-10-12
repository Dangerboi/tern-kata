# From 140 columns the Issue list and the detail each sit in a bordered panel
# (a Tern card): the focused one has the accent ring (class "selected"), the
# other the plain one, and they swap with focus. The top bar stays above both.
# Under 140 columns nothing is bordered. The step resizes the window and moves
# focus; it puts both back. Needs the seeded Issues.
read -r zatn_width zatn_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')
# Expects the list ($1) and detail ($2) panels to be bordered cards, each ring accent (1) or plain (0).
expect_panels() {
  expect_tree "[data-id='main.columns.list']" ".ok and .count == 1 and (.nodes[0].class | contains(\"sf-card\")) and (.nodes[0].class | contains(\"selected\")) == ($1 == 1)"
  expect_tree "[data-id='main.columns.detail']" ".ok and .count == 1 and (.nodes[0].class | contains(\"sf-card\")) and (.nodes[0].class | contains(\"selected\")) == ($2 == 1)"
}

ctl resize 1200 "$zatn_height"
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1 and (.nodes[0].class | contains("sf-card"))'
ctl tree "[data-id='main.columns.list'],[data-id='main.columns.detail']" | jq -r '.nodes | map(.class | contains("selected")) | @csv' >"$tmp/panels-focus.csv"
case $(<"$tmp/panels-focus.csv") in
  true,false) zatn_start=list; expect_panels 1 0; ctl key enter; expect_panels 0 1; ctl key esc ;;
  false,true) zatn_start=detail; expect_panels 0 1; ctl key esc; expect_panels 1 0; ctl key enter ;;
  *) printf 'Expected exactly one accent panel, saw %s\n' "$(<"$tmp/panels-focus.csv")" >&2; return 1 ;;
esac
if [[ $zatn_start == list ]]; then expect_panels 1 0; else expect_panels 0 1; fi

# The top bar is above both panels, not a card and not inside one.
ctl tree "[data-id='main.topbar'],[data-id='main.columns.list'],[data-id='main.columns.detail']" >"$tmp/panels-tree.json"
jq -e '.count == 3 and (.nodes[0].class | contains("sf-card") | not) and ([.nodes[1:][].rect[1]] | all(. >= $bar)) ' --argjson bar "$(jq '.nodes[0].rect | .[1] + .[3]' "$tmp/panels-tree.json")" "$tmp/panels-tree.json" >/dev/null

# One column from 80 to 139 columns shows the focused column, with no card anywhere.
ctl resize 800 "$zatn_height"
if [[ $zatn_start == list ]]; then zatn_shown=list zatn_hidden=detail zatn_move=enter; else zatn_shown=detail zatn_hidden=list zatn_move=esc; fi
expect_tree "[data-id='main.columns.$zatn_shown']" '.ok and .count == 1 and (.nodes[0].class | contains("sf-card") | not)'
expect_tree "[data-id='main.columns.$zatn_hidden']" '.ok and .count == 0'
ctl key "$zatn_move"
expect_tree "[data-id='main.columns.$zatn_hidden']" '.ok and .count == 1 and (.nodes[0].class | contains("sf-card") | not)'
expect_tree "[data-id='main.columns.$zatn_shown']" '.ok and .count == 0'
if [[ $zatn_start == list ]]; then ctl key esc; else ctl key enter; fi
expect_tree "[data-id='main.columns.$zatn_shown']" '.ok and .count == 1'

ctl resize "$zatn_width" "$zatn_height"
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'
