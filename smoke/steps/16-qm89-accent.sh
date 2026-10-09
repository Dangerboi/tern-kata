# The selected Issue row is painted in the theme's accent fill (var(--accent-fill), never a fixed colour):
# its computed background is opaque and differs from the bare and striped rows, its text is white with a WCAG
# contrast of at least 4.5 against that background, and the highlight moves with `j`. Colours settle after
# Tern's transition, so each check polls.
accent_expect() {
  local program='[.elements[] | select(.path | split(">") | last | test("^div\\.sf\\.sf-item")) | {sel: (.path | split(">") | last | contains(".sel")), bg: (.backgroundColor | [scan("[0-9.]+")] | map(tonumber)), fg: (.color | [scan("[0-9.]+")] | map(tonumber))}] as $r
    | def lin: . / 255 | if . <= 0.03928 then . / 12.92 else pow((. + 0.055) / 1.055; 2.4) end;
      def lum: 0.2126 * (.[0] | lin) + 0.7152 * (.[1] | lin) + 0.0722 * (.[2] | lin);
      ($r | map(select(.sel))) as $s
    | ($s | length) == 1 and ($s[0].bg[3] // 1) > 0.98 and $s[0].fg == [255, 255, 255]
      and (($r | map(select(.sel | not) | (.bg[3] // 1)) | all(. < 0.1)))
      and ((1.05 / (($s[0].bg | lum) + 0.05)) >= 4.5)'
  for ((attempt = 0; attempt < 200; attempt++)); do
    ctl dump "[data-id^='main.columns.list.issues.']" >"$tmp/accent-dump.json"
    if jq -e "$program" "$tmp/accent-dump.json" >/dev/null; then return; fi
    sleep 0.1
  done
  jq -c '.elements[] | {path: (.path | split(">") | last), bg: .backgroundColor, color}' "$tmp/accent-dump.json" >&2
  printf 'The selected row is not an accent-filled, readable row\n' >&2
  return 1
}
ctl hover "[data-id='main.topbar']" >/dev/null
accent_expect
ctl key g
ctl key j
accent_expect
ctl key k
accent_expect
