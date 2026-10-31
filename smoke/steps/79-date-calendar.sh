# ctrl+d in a date field of the Create card opens a calendar beside it: the arrows move its cursor, enter picks the day into
# the field; esc closes the calendar without touching the field. A click on a day is covered by a driver test.
ctl key n
expect_tree "[data-id='layer.create.title']" '.count == 1'
for _ in 1 2 3 4 5; do ctl key tab; done
ctl key ctrl+d
if ! expect_tree "[data-id='layer.create-calendar.calendar']" '.count == 1' 2>/dev/null; then
  ctl key meta+d
fi
expect_tree "[data-id='layer.create-calendar.calendar']" '.count == 1'
ctl key escape
expect_tree "[data-id='layer.create-calendar.calendar']" '.count == 0'
expect_tree "[data-id='layer.create']" '.count == 1'
ctl key ctrl+d
if ! expect_tree "[data-id='layer.create-calendar.calendar']" '.count == 1' 2>/dev/null; then
  ctl key meta+d
fi
ctl key right
ctl key enter
expect_tree "[data-id='layer.create-calendar.calendar']" '.count == 0'
expect_tree "[data-id='layer.create.scheduled']" '.count == 1 and (.nodes[0].text | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}insert$"))'
ctl key escape
ctl key y
expect_tree "[data-id='layer.create']" '.count == 0'
