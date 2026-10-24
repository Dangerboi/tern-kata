# The Daemons Page (D) lists the daemons kata's config.toml names with kata tui's columns, and Enter switches this Pane to one:
# another running daemon (a remote entry), a stopped local profile (started first), and back to the current home. A daemon that
# cannot be reached leaves the Pane where it was. Own daemons and homes; the catalog is restored and every daemon stopped.
dm_home="$tmp/daemons-home"
dm_other="$tmp/daemons-other"
dm_profile="$tmp/daemons-profile"
dm_ws_other="$tmp/daemons-ws-other"
dm_ws_profile="$tmp/daemons-ws-profile"
mkdir -p "$dm_other" "$dm_profile" "$dm_ws_other" "$dm_ws_profile"
dm_port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
KATA_HOME="$dm_other" kata daemon start --listen "127.0.0.1:$dm_port" >/dev/null
KATA_HOME="$dm_other" kata --workspace "$dm_ws_other" init --project other-daemon-project --json >/dev/null
# The profile's daemon runs once to create its database, then stops: the switch has to start it again.
KATA_HOME="$dm_profile" kata daemon start >/dev/null
KATA_HOME="$dm_profile" kata --workspace "$dm_ws_profile" init --project stopped-profile-project --json >/dev/null
dm_uid=$(KATA_HOME="$dm_profile" kata daemon diagnose --json | jq -r '.observed_instance_uid')
KATA_HOME="$dm_profile" kata daemon stop >/dev/null

cp "$KATA_HOME/config.toml" "$dm_home.config"
cat >>"$KATA_HOME/config.toml" <<EOF

[[daemon]]
name = "smoke-home"
local = true

[[daemon]]
name = "smoke-other"
url = "http://127.0.0.1:$dm_port"
allow_insecure = true

[[daemon]]
name = "smoke-profile"
local = true
home = "$dm_profile"
instance_uid = "$dm_uid"

[[daemon]]
name = "smoke-broken"
url = "http://127.0.0.1:1"
allow_insecure = true
EOF

ctl key D
expect_tree "[data-id='main.columns.list.summary']" '.ok and (.nodes[0].text == "4 daemons")'
expect_tree "[data-id='main.columns.list.header']" '.ok and (.nodes[0].text | startswith("Daemon") and contains("Endpoint") and contains("Auth") and contains("State"))'
expect_tree "[data-id='main.columns.list.daemons.smoke-home.row']" '.ok and (.nodes[0].text | contains("local") and contains("no token"))'
expect_tree "[data-id='main.columns.list.daemons.smoke-other.row']" '.ok and (.nodes[0].text | contains("127.0.0.1:'"$dm_port"'") and contains("no token"))'
expect_tree "[data-id='main.columns.list.footer']" '.ok and (.nodes[0].text == "smoke-home · local")'

# An unreachable daemon: the Pane says why and stays on the daemon it was on.
ctl key j
ctl key j
ctl key j
expect_tree "[data-id='main.columns.list.footer']" '.ok and (.nodes[0].text | startswith("smoke-broken · 127.0.0.1:1") and endswith("allow_insecure"))'
ctl key enter
expect_tree "[data-id\$='.notice']" '.ok and .count == 1 and (.nodes[0].text | startswith("daemon \"smoke-broken\": "))'
expect_tree "[data-id='main.topbar.right.daemon']" '.ok and (.nodes[0].text | startswith("Daemon: local"))'

# A second running daemon: a successful switch leaves the Page, starts the Pane over on that daemon (its own Projects, not the
# smoke Project) and names it in the top bar.
ctl key k
ctl key k
ctl key enter
expect_tree "[data-id='main.topbar.right.daemon']" '.ok and (.nodes[0].text | startswith("Daemon: smoke-other · remote"))'
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id='main.topbar.left.heading']" '.ok and (.nodes[0].text != "Project: smoke")'
ctl key D
expect_tree "[data-id='main.columns.list.daemons.smoke-other.row']" '.ok and (.nodes[0].text | endswith("current"))'

# A stopped local profile is started by the switch: the cursor starts on the current daemon, so j is the profile.
ctl key j
ctl key enter
expect_tree "[data-id='main.topbar.right.daemon']" '.ok and (.nodes[0].text | startswith("Daemon: smoke-profile · local"))'

# Back to the current home: the workspace's Project is bound again.
ctl key D
ctl key g
ctl key enter
expect_tree "[data-id='main.topbar.right.daemon']" '.ok and (.nodes[0].text | startswith("Daemon: smoke-home · local"))'
expect_tree "[data-id='main.topbar.left.heading']" '.ok and .nodes[0].text == "Project: smoke"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 3'

cp "$dm_home.config" "$KATA_HOME/config.toml"
KATA_HOME="$dm_other" kata daemon stop >/dev/null
KATA_HOME="$dm_profile" kata daemon stop >/dev/null
