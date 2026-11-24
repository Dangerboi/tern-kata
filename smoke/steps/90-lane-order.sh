# A card dragged up or down inside its Lane gives the Lane a custom order: the header says so, lanes.toml holds the order, a
# relaunched Pane shows it, a card dropped from another Lane on the top card lands above it, the Lane view's header says so too,
# and the header menu's Reset order returns the list's order. Own Issues in needs-info and needs-triage; they stay open.
lo_dir="$tmp/workspace"
# Tern's headless serve gives the Pane its own home, so its config folder is read from the running Pane, not from $XDG_CONFIG_HOME.
lo_pane_config() { local pid; pid=$(pgrep -n -f "$tmp/tern-kata") || return 1; tr '\0' '\n' <"/proc/$pid/environ" | sed -n 's/^XDG_CONFIG_HOME=//p'; }
lo_file=
lo_uids=()
for lo_name in A B C; do
  kata --workspace "$lo_dir" create "Smoke order $lo_name" --label needs-info --json >"$tmp/lo-$lo_name.json"
  lo_uids+=("$(jq -r '.issue.uid' "$tmp/lo-$lo_name.json")")
done
lo_lane="[data-id='main.columns.list.board.lane2']"
lo_head="$lo_lane [data-id='main.columns.list.board.lane2.head']"
lo_card() { printf "[data-id='main.columns.list.board.lane2.cards.%s']" "$1"; }
lo_rects() { local uid; for uid in "${lo_uids[@]}"; do rect "$(lo_card "$uid")"; done; }
# lo_top_down: the three uids, top card first.
lo_top_down() {
  local uid y
  for uid in "${lo_uids[@]}"; do
    y=$(ctl tree "$(lo_card "$uid")" | jq -er '.nodes[0].rect[1]') || return 1
    printf '%s %s\n' "$y" "$uid"
  done | sort -n | cut -d' ' -f2
}
lo_expect_cards() {
  local uid
  for uid in "${lo_uids[@]}"; do expect_tree "$(lo_card "$uid")" '.ok and .count == 1'; done
}
lo_first_is() { [[ $(lo_top_down | head -n 1) == "$1" ]]; }
lo_order_in_file() { [[ -f $lo_file ]] && grep -qF "${lo_uids[0]}" "$lo_file"; }
lo_header_custom() { ctl tree "$lo_head" | jq -e '.count == 1 and (.nodes[0].text | contains("custom order"))' >/dev/null; }
lo_header_plain() { ctl tree "$lo_head" | jq -e '.count == 1 and (.nodes[0].text | contains("custom order") | not)' >/dev/null; }

ctl key 5
wait_for lo_pane_config
lo_file="$(lo_pane_config)/tern-kata/lanes.toml"
lo_expect_cards
wait_settled lo_rects
mapfile -t lo_before < <(lo_top_down)
lo_dragged=${lo_before[2]}
lo_header_plain

# The bottom card dropped on the upper half of the top card lands first.
read -r tx ty tw th < <(rect "$(lo_card "${lo_before[0]}")")
drag "$(lo_card "$lo_dragged")" $((tx + tw / 2)) $((ty + th / 4))
wait_for lo_first_is "$lo_dragged"
wait_settled lo_rects
wait_for lo_header_custom
wait_for lo_order_in_file
[[ $(grep -oE "${lo_uids[0]}|${lo_uids[1]}|${lo_uids[2]}" "$lo_file" | head -n 1) == "$lo_dragged" ]]
grep -q 'order' "$lo_file"
mapfile -t lo_after < <(lo_top_down)

# A relaunched Pane keeps the order.
quit_app
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
ctl key 5
wait_for lo_pane_config
lo_file="$(lo_pane_config)/tern-kata/lanes.toml"
lo_expect_cards
wait_settled lo_rects
lo_first_is "$lo_dragged"
lo_header_custom
[[ $(lo_top_down) == "$(printf '%s\n' "${lo_after[@]}")" ]]

# A card from needs-triage dropped on the upper part of the custom-ordered Lane's top card lands directly above that card.
kata --workspace "$lo_dir" create "Smoke order D" --label needs-triage --json >"$tmp/lo-D.json"
lo_d=$(jq -r '.issue.uid' "$tmp/lo-D.json")
lo_d_card="[data-id='main.columns.list.board.lane1.cards.$lo_d']"
expect_tree "$lo_d_card" '.ok and .count == 1'
lo_d_rect() { rect "$lo_d_card"; }
wait_settled lo_d_rect
wait_settled lo_rects
read -r tx ty tw th < <(rect "$(lo_card "${lo_after[0]}")")
drag "$lo_d_card" $((tx + tw / 2)) $((ty + th / 4))
lo_d_in_lane() { kata --workspace "$lo_dir" show "$lo_d" --json | jq -e '.issue.labels | index("needs-info")' >/dev/null; }
wait_for lo_d_in_lane
lo_d_card="[data-id='main.columns.list.board.lane2.cards.$lo_d']"
expect_tree "$lo_d_card" '.ok and .count == 1'
wait_settled lo_d_rect
# The four cards of the Lane by rect y: the new one first, then the three in their custom order.
lo_four_down() {
  local uid y
  for uid in "$lo_d" "${lo_uids[@]}"; do
    y=$(ctl tree "$(lo_card "$uid")" | jq -er '.nodes[0].rect[1]') || return 1
    printf '%s %s\n' "$y" "$uid"
  done | sort -n | cut -d' ' -f2
}
lo_d_on_top() { [[ $(lo_four_down) == "$(printf '%s\n' "$lo_d" "${lo_after[@]}")" ]]; }
wait_for lo_d_on_top

# The Lane view's header carries the custom order label too.
wait_settled lo_rects
ctl click "$(lo_card "$lo_d")"
ctl key Enter
expect_tree "[data-id='main.columns.list.lane']" '.ok and .count == 1 and (.nodes[0].text | contains("custom order"))'
ctl key Escape
expect_tree "[data-id='main.columns.list.lane']" '.ok and .count == 0'
expect_tree "$lo_head" '.ok and .count == 1'
wait_settled lo_rects

# Reset order in the header's menu returns the list's order and drops the stored one.
ctl click "$lo_head" right
expect_tree "[role='menu']" '.ok and .count == 1 and (.nodes[0].text | contains("Reset order"))'
ctl tree "[role='menuitem']" | jq -er '[.nodes[] | select(.text | startswith("Reset order"))][0].rect | "\(.[0] + .[2] / 2) \(.[1] + .[3] / 2)"' >"$tmp/lo-reset.xy"
# shellcheck disable=SC2046
ctl click $(<"$tmp/lo-reset.xy")
wait_for lo_header_plain
wait_settled lo_rects
[[ $(lo_top_down) == "$(printf '%s\n' "${lo_before[@]}")" ]]
lo_order_gone() { ! grep -qF "${lo_uids[0]}" "$lo_file"; }
wait_for lo_order_gone

quit_app
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
