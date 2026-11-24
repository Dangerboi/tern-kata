# A card dragged up or down inside its Lane gives the Lane a custom order: the header says so, lanes.toml holds the order, a
# relaunched Pane shows it, and the header menu's Reset order returns the list's order. Own Issues in needs-info; they stay open.
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
# lo_rect <selector>: the first match's "x y w h", whole pixels.
lo_rect() { ctl tree "$1" | jq -er '.nodes[0].rect | map(floor) | "\(.[0]) \(.[1]) \(.[2]) \(.[3])"'; }
lo_rects() { local uid; for uid in "${lo_uids[@]}"; do lo_rect "$(lo_card "$uid")"; done; }
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
read -r tx ty tw th < <(lo_rect "$(lo_card "${lo_before[0]}")")
read -r bx by bw bh < <(lo_rect "$(lo_card "$lo_dragged")")
printf 'down %s %s\nmove %s %s\nmove %s %s\nmove %s %s\nup %s %s\n' \
  $((bx + bw / 2)) $((by + bh / 2)) $((bx + bw / 2 + 6)) $((by + bh / 2 - 6)) \
  $((tx + tw / 2)) $(((by + ty) / 2)) $((tx + tw / 2)) $((ty + th / 4)) $((tx + tw / 2)) $((ty + th / 4)) >"$tmp/lo-drag.ctl"
ctl --file "$tmp/lo-drag.ctl"
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
