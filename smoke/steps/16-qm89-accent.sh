# The selected Issue row is painted in the theme's accent fill (var(--accent-fill), never a fixed colour):
# its computed background equals the one Tern paints its own accent fill with (the drop marker .tn-drop-l) and
# differs from the bare and striped rows, its text is white with a WCAG contrast of at least 4.5 against it,
# also while the pointer hovers the row, and the highlight moves with `j` and back with `k`. Colours settle
# after Tern's transition, so each check polls (`accent_expect first`, `same RECT` and `other RECT` also wait for the
# selected row to be the top row, or to be, or not be, at RECT). accent_expect prints the selected row's rect.
accent_expect() {
  local mode=${1:-} want=${2:-}
  local program='[.elements[] | select(.path | split(">") | last | test("^div\\.sf\\.sf-item")) | {rect: .rect, sel: (.path | split(">") | last | contains(".sel")), bg: (.backgroundColor | [scan("[0-9.]+")] | map(tonumber)), fg: (.color | [scan("[0-9.]+")] | map(tonumber))}] as $r
    | def lin: . / 255 | if . <= 0.03928 then . / 12.92 else pow((. + 0.055) / 1.055; 2.4) end;
      def lum: 0.2126 * (.[0] | lin) + 0.7152 * (.[1] | lin) + 0.0722 * (.[2] | lin);
      ($r | map(select(.sel))) as $s
    | ($s | length) == 1 and ($s[0].bg[3] // 1) > 0.98 and $s[0].fg == [255, 255, 255]
      and (($r | map(select(.sel | not) | (.bg[3] // 1)) | all(. < 0.1)))
      and ((1.05 / (($s[0].bg | lum) + 0.05)) >= 4.5)
      and $s[0].bg == $ref
      and (if $mode == "" then true elif $mode == "first" then $r[0].sel else (($s[0].rect | tostring) == $want) == ($mode == "same") end)'
  local program_path='[.elements[] | select(.path | split(">") | last | test("^div\\.sf\\.sf-item")) | select(.path | split(">") | last | contains(".sel")) | .rect | tostring] | .[0]'
  local ref
  ref=$(ctl dump ".tn-drop-l" | jq -c '.elements[0].backgroundColor | [scan("[0-9.]+") | tonumber]')
  for ((attempt = 0; attempt < 200; attempt++)); do
    ctl dump "[data-id^='main.columns.list.issues.']" >"$tmp/accent-dump.json"
    if jq -e --argjson ref "$ref" --arg mode "$mode" --arg want "$want" "$program" "$tmp/accent-dump.json" >/dev/null; then
      jq -r "$program_path" "$tmp/accent-dump.json"
      return
    fi
    sleep 0.1
  done
  jq -c '.elements[] | {path: (.path | split(">") | last), bg: .backgroundColor, color}' "$tmp/accent-dump.json" >&2
  printf 'The selected row is not an accent-filled, readable row\n' >&2
  return 1
}
ctl hover "[data-id='main.topbar']" >/dev/null
ctl key g
first=$(accent_expect first)
ctl key j
accent_expect other "$first" >/dev/null
ctl key k
accent_expect same "$first" >/dev/null
# The hover must not beat the highlight.
ctl hover ".sf-item.sel[data-id^='main.columns.list.issues.']" >/dev/null
accent_expect same "$first" >/dev/null
ctl hover "[data-id='main.topbar']" >/dev/null
