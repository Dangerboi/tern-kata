# W on the Jobs Page opens the Worker card. With no worker.toml it offers Set up with the fields prefilled as `worker init` would;
# a save is refused until the trust choice is yes, then writes a worker.toml that loads, and the card says the plugin starts the
# worker. Run it with throwaway XDG_CONFIG_HOME and XDG_STATE_HOME: it writes the worker config there and removes it again.
: "${XDG_CONFIG_HOME:?run this step with a throwaway XDG_CONFIG_HOME}" "${XDG_STATE_HOME:?run this step with a throwaway XDG_STATE_HOME}"
wc_file="$XDG_CONFIG_HOME/tern-kata/worker.toml"
[[ ! -e "$wc_file" ]]

ctl key 3
expect_tree "[data-id='main.columns.list.jobs']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.worker']" '.ok and .count == 1 and (.nodes[0].text | contains("press W to set one up"))'
ctl key W
expect_tree "[data-id='layer.worker']" '.ok and .count == 1 and (.nodes[0] | [.. | objects | .text? // empty] | join(" ") | contains("none yet") and contains("Project: smoke"))'
expect_tree "[data-id='layer.worker.executor']" '.ok and .count == 1 and (.nodes[0].text | length > 0)'
expect_tree "[data-id='layer.worker.command']" '.ok and .count == 1 and (.nodes[0].text | startswith("omp"))'

# Without the trust choice nothing is written and the refusal names it.
ctl key Ctrl+o
expect_tree "[data-id='layer.worker.error']" '.ok and .count == 1 and (.nodes[0].text | contains("Trust"))'
[[ ! -e "$wc_file" ]]

ctl click "[data-id='layer.worker.trust.choices.1']"
ctl key Ctrl+o
expect_tree "[data-id='layer.worker.saved']" '.ok and .count == 1 and (.nodes[0].text | contains("worker.toml written"))'
expect_tree "[data-id='layer.worker.next']" '.ok and .count == 1 and (.nodes[0].text | contains("starts the worker within"))'
[[ -s "$wc_file" ]]
grep -q 'name = "smoke"' "$wc_file"
"$tmp/tern-kata" worker doctor >"$tmp/worker-doctor.txt" 2>&1 || true
! grep -qi 'unknown key\|is required' "$tmp/worker-doctor.txt"
ctl key Escape
expect_tree "[data-id='layer.worker']" '.ok and .count == 0'

# The banner now sees a file that lists the Project.
ctl key ctrl+r
expect_tree "[data-id='main.columns.list.worker']" '.ok and .count == 1 and (.nodes[0].text | contains("W shows the setup") or contains("running"))'
rm -f "$wc_file"
ctl key 1
