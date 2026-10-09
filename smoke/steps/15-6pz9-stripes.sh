# Every other Issue row is shaded by the plugin's style sheet, through a role on the row: the first
# row bare, the second shaded, and so on, the header never. The selected row keeps its highlight,
# the stripes stay alternating after a fold, and the shade is the theme's own text colour at low
# opacity (never a fixed colour). Rows show no labels. The fixtures are deleted afterwards.
kata --workspace "$tmp/workspace" create "Smoke stripe parent" --label stripe-label --json >"$tmp/stripe-parent.json"
stripe_parent=$(jq -r '.issue.uid' "$tmp/stripe-parent.json")
stripe_children=() stripe_shorts=("$(jq -r '.issue.short_id' "$tmp/stripe-parent.json")")
for n in 1 2; do
  kata --workspace "$tmp/workspace" create "Smoke stripe child $n" --parent "$stripe_parent" --json >"$tmp/stripe-child.json"
  stripe_children+=("$(jq -r '.issue.uid' "$tmp/stripe-child.json")")
  stripe_shorts+=("$(jq -r '.issue.short_id' "$tmp/stripe-child.json")")
done

# stripe_expect ROWS: ROWS Issue rows are drawn and their computed backgrounds alternate: bare on even
# positions, the theme's text colour at 5% on odd ones, the selected row (its own highlight) excepted.
stripe_expect() {
  ctl hover "[data-id='main.topbar']" >/dev/null
  local program='[.elements[] | select(.path | split(">") | last | test("^div\\.sf\\.sf-item")) | {sel: (.path | split(">") | last | contains(".sel")), bg: .backgroundColor, stripe: (.color | sub("rgb\\("; "rgba(") | sub("\\)$"; ", 0.05)"))}] as $r
    | ($r | length) == '"$1"'
    and all(range(0; $r | length); . as $i | $r[$i] | if .sel then .bg != "rgba(0, 0, 0, 0)" and .bg != .stripe elif $i % 2 == 1 then .bg == .stripe else .bg == "rgba(0, 0, 0, 0)" end)'
  for ((attempt = 0; attempt < 200; attempt++)); do
    ctl dump "[data-id^='main.columns.list.issues.']" >"$tmp/stripe-dump.json"
    if jq -e "$program" "$tmp/stripe-dump.json" >/dev/null; then return; fi
    sleep 0.1
  done
  jq -c '.elements[] | {path: (.path | split(">") | last), bg: .backgroundColor, color}' "$tmp/stripe-dump.json" >&2
  printf 'Row stripes are not alternating over %s rows\n' "$1" >&2
  return 1
}

expect_tree "[data-id\$='.$stripe_parent']" '.ok and .count == 1'
stripe_rows=$(ctl tree "[data-id^='main.columns.list.issues.']" | jq '.count')
((stripe_rows >= 4)) || { printf 'Expected at least four Issue rows, got %s\n' "$stripe_rows" >&2; return 1; }
stripe_expect "$stripe_rows"
# The header is a header, not a shaded row; rows carry no label text.
expect_tree "[data-id='main.columns.list.header']" '.ok and .count == 1'
ctl dump "[data-id='main.columns.list.header']" | jq -e '.elements[0].backgroundColor == "rgba(0, 0, 0, 0)"' >/dev/null
expect_tree "[data-id^='main.columns.list.issues.']" '.ok and ([.nodes[].text | contains("stripe-label")] | any | not)'
expect_tree "[data-id='main.columns.list.issues.$stripe_parent']" '.ok and .count == 1'

# Moving the selection onto a shaded position keeps the highlight, not the shade: the rows stay alternating.
ctl key j
stripe_expect "$stripe_rows"
ctl key k

# Unfolding the parent puts its two children among the rows and the stripes follow; folding takes them away again.
ctl click "[data-id\$='.$stripe_parent']"
expect_tree "[data-id\$='.${stripe_children[1]}']" '.ok and .count == 1'
stripe_expect $((stripe_rows + 2))
ctl click "[data-id\$='.$stripe_parent']"
expect_tree "[data-id\$='.${stripe_children[1]}']" '.ok and .count == 0'
stripe_expect "$stripe_rows"

for i in "${!stripe_children[@]}"; do
  kata --workspace "$tmp/workspace" delete "${stripe_children[$i]}" --force --confirm "DELETE smoke#${stripe_shorts[$((i + 1))]}" --json >/dev/null
done
kata --workspace "$tmp/workspace" delete "$stripe_parent" --force --confirm "DELETE smoke#${stripe_shorts[0]}" --json >/dev/null
expect_tree "[data-id\$='.$stripe_parent']" '.ok and .count == 0'
