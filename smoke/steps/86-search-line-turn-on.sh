# kata searched by words with no reason because the smoke daemon has no embeddings (and lets this account change them), so the
# search line says search by meaning is off on this daemon and offers ctrl+o. ctrl+o opens the Search by meaning card's question;
# n closes it and leaves the search where it was. Nothing is downloaded: y is never pressed.
meaning_line="[data-id='main.columns.list.search-line']"
embeddings_card="[data-id='layer.embeddings']"
# Headless Tern delivers ctrl+t and ctrl+o as meta+t and meta+o, which the key table binds too.
ctl key /
ctl key ctrl+t
for letter in z e b r a; do ctl key "$letter"; done
ctl key enter
expect_tree "$meaning_line" '.ok and .count == 1 and (.nodes[0].text == "kata searched by words: search by meaning is off on this daemon")'
expect_tree "[data-id='main.columns.list.search-turn-on']" '.ok and .count == 1'

ctl key ctrl+o
expect_tree "$embeddings_card" '.ok and .count == 1'
expect_tree "[data-id='layer.embeddings.question']" '.ok and .count == 1 and (.nodes[0].text | startswith("Turn on search by meaning? kata ") and endswith(" y/n"))'

# n sends nothing: the card goes and the search line and its button stay.
ctl key n
expect_tree "$embeddings_card" '.ok and .count == 0'
expect_tree "$meaning_line" '.ok and .count == 1 and (.nodes[0].text == "kata searched by words: search by meaning is off on this daemon")'
ctl key escape
ctl key escape
expect_tree "$meaning_line" '.ok and .count == 0'
