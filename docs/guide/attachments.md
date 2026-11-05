# Attachments

Attach a screenshot, log or any other file to an Issue so whoever picks it up sees what you saw. kata stores the file: its bytes go to [Docbank](https://github.com/kenn-io/docbank) and the Issue keeps a reference to them, so every agent and person who can read the Issue can download the file, from any machine.

![The Attachments tab of "Add Apple Pay to checkout" listing a screenshot and a log, each with its media type and size](../images/attachments.png)

This needs kata v0.21 and a daemon with Docbank configured (see [Setting Docbank up](#setting-docbank-up)). Without it the Pane says so plainly and nothing can be attached.

## In a Pane

- `@` on the selected Issue (in the list, on the Board, in the Lane view or in the Issue detail) asks for a path, quoted, starting with `~` or relative to the directory, and uploads the file under its own name. A refusal or a failure shows in the form. A file may be up to 256 MiB.
- `U` lists the Issue's Attachments by name: `down` and `up` move and `enter` unlinks the one chosen, with no further confirmation. You can also type an attachment's name in full. Unlinking removes the Issue's reference only; the bytes stay in Docbank, because kata has no call that deletes them. Two Attachments with one name show with the last characters of their ids to tell them apart. `U` on an Issue with none says so.
- The Attachments tab of the Issue detail lists the Issue's Attachments with their name, media type and size. `enter` on one downloads it into a cache folder and, if it is an image, video, audio, PDF or text file by its name, opens it with the program your OS has for that type; the next `enter` reuses the download. tern-kata opens images (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.bmp`, `.heic`, `.tif`, `.tiff`), video (`.mp4`, `.mov`, `.webm`, `.mkv`, `.avi`, `.m4v`), audio (`.mp3`, `.wav`, `.ogg`, `.flac`, `.m4a`), `.pdf` and plain text (`.txt`, `.md`, `.log`, `.csv`, `.json`), in any letter case, judged by the name exactly as stored: `report.bat.` or `x.png ` do not count. Any other file is only saved, never opened, because what the OS does with a file it knows (run it, follow a link inside it) cannot be told from its name and anyone who can write the Project can name an Attachment `report.bat`. The Pane then says where it is saved and that tern-kata opens only images, video, audio, PDFs and text: "Cannot open report.bat: saved at <path> and not opened: tern-kata opens only images, video, audio, PDFs and text". Open such a file yourself if you trust it. The download never gets an execute permission.

## Citing an attachment when closing

`x` opens the Close card. For a close with the reason `done` on an Issue that has Attachments, the card has an Attachments row after External account: the Issue's Attachments by name, with the last characters of the id after a repeated name. `tab` moves to it, `left` and `right` move the `▸` mark along the names, `space` (or a click on a name) cites the one under the mark and a second `space` takes it back; a cited name shows in brackets. Cite one or more. An Attachment counts as evidence like a commit, PR, test, reviewed paths or external account, so a done close needs no other evidence when it cites one.

The row is not offered for `wontfix`, `duplicate`, `superseded` or `audit-no-change`, which kata does not accept an Attachment for, or for an Issue with no Attachments. Changing the reason away from `done` hides the row and a close with another reason does not send the picks. Quick done sends no evidence, so it cites none. The Close card that completes an undone reopen offers the row too.

kata freezes the Attachment's name, media type, size and Docbank version into the close, so the Events tab still names the file after the Attachment is unlinked. A close line reads `closed: done`, the message, then `(attachment report.pdf)`; the name is followed by `[removed]` once the Issue no longer has the Attachment and by `[changed]` when the Issue now has another version of that file under the same id. A close recorded without that frozen copy reads the name from the Issue's current Attachment, and "Attachment removed" when it has none. This needs a kata daemon that speaks HTTP API 0.39.0 or newer (kata v0.21). `kata close <ref> --done --evidence attachment:<full-id>` records the same thing from a shell, and `--docbank <node-id|/path>` also links a Docbank file to the Issue first.

The cache folder is `tern-kata/attachments/<Issue UID>/<attachment id>/<version>/<name>` under your user cache directory (`$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows). `<version>` is a short hash of the Attachment's Docbank version, so each version of an Attachment gets its own folder and a changed file is downloaded again instead of read from the old copy. It is yours to delete.

A daemon without Docbank is said so in the Attachments tab and as the error of `@`. Other failures are in words too: Docbank refusing kata's API key, Docbank not reachable, a file over the limit, a name kata refuses (empty, over 255 bytes, or holding `/`, `\` or a control character), content Docbank no longer has, and an Issue that is a read-only copy from another kata instance.

## From a shell or for an agent

kata has its own commands; tern-kata has none for attachments (`tern-kata attach` and `detach` no longer exist):

```sh
kata docbank attachments add <ref> <file>...   # upload; prints each attachment id
kata docbank attachments ls <ref>              # id, name, media type and size
kata docbank attachments get <ref> <id> -o <path> [--force]
kata docbank attachments rm <ref> <id>         # unlink; the bytes stay in Docbank
```

`<id>` may be a unique suffix of the id. See `kata docbank attachments --help` for the rest. Agents read the same commands from the `kata` skill.

An Attachment is one meta key on the Issue, `attachment.<id>`, holding `{"name", "media_type", "size", "docbank_version"}`, visible in `kata show --json`.

## Setting Docbank up

You do this once, for the kata daemon. `tern-kata docbank setup` does it, for Docbank on this machine and for one on another machine (below). kata only talks to Docbank over HTTP: it never starts Docbank, so Docbank needs a fixed address, a fixed API key and a process that keeps running. For Docbank on this machine the tern-kata plugin keeps it running (below).

Choose a vault that every reader of your Issues may open. kata puts files in a folder of that vault; the folder only organises them and is no access control, so whoever can read an Issue can download its attachments.

### Docbank on this machine

Install [Docbank](https://github.com/kenn-io/docbank), then run:

```sh
tern-kata docbank setup
```

- It gives Docbank a `[server]` section in `<DOCBANK_HOME>/config.toml` (`~/.docbank/config.toml` unless `DOCBANK_HOME` names another folder) with `bind_addr = "127.0.0.1"`, a fixed `api_port` (`--port`, default 8080) and an `api_key` of 32 random bytes as hex, and stores the same key in `<KATA_HOME>/docbank.key` (mode 0600). The key is never printed.
- It appends the section as text, keeping every other byte of the file, and creates the file and `DOCBANK_HOME` when they are missing. An existing `[server]` with `api_port` and `api_key` is reused (the key is copied to the key file when that is missing) and a different `--port` is refused; a `[server]` missing one of them is refused, naming what to add.
- It refuses before writing anything when `docbank` is not on `PATH`, when another process listens on the port, or when kata's `config.toml` already has a different `[docbank]` section.
- It runs `docbank daemon restart` and requires `docbank daemon status` to name `127.0.0.1:<port>`; otherwise it stops, shows Docbank's output and leaves kata's config alone.
- Then it does what the remote form does with `url = "http://127.0.0.1:<port>"` and `api_key_file = <KATA_HOME>/docbank.key`: check Docbank with the key, append the `[docbank]` section, ask before restarting the kata daemon (`--yes`, `--no-restart`), and check that attachments are on. Running it again says "already set up" and restarts nothing.
- `--folder` and the route flags work as in the remote form below. `--key-file` and `--key-env` are for the remote form only, and `--port` for this one.
- It ends by saying that the plugin keeps Docbank running.

#### The plugin keeps Docbank running

While Tern runs, the plugin's host half checks every 10 seconds whether Docbank runs and, when none does, runs `docbank daemon start` (Docbank's own detached start, from your home folder). A failed start is retried after 1 s, then 2 s, doubling to 60 s, and the backoff starts over once Docbank has stayed up for a minute. The Tern log gets one line when it starts Docbank and one when a start fails, not one per check. A Docbank that is already running, however it was started, is left alone.

The plugin does this exactly when kata's `[docbank]` `url` is on this machine (`127.0.0.1`, `localhost` or `::1`), Docbank's `[server]` `api_port` in `<DOCBANK_HOME>/config.toml` is that url's port, and the file `docbank.manual` is not in tern-kata's config folder (`~/.config/tern-kata` on Linux). `tern-kata docbank supervised` prints `on` and exits 0 when that holds, and exits 1 with the reason on standard error when not. A Docbank on another machine is never started.

To run Docbank your own way, turn this off:

```sh
tern-kata docbank supervise off   # creates docbank.manual; a running Docbank keeps running
tern-kata docbank supervise on    # removes it
```

The same by hand, for reference:

1. Install [Docbank](https://github.com/kenn-io/docbank). Its vault is `~/.docbank` unless `DOCBANK_HOME` names another folder; the first `docbank` command creates it.
2. Make an API key and keep it in a file only you can read:

   ```sh
   openssl rand -hex 32 > ~/.kata/docbank.key && chmod 600 ~/.kata/docbank.key
   ```

3. Give Docbank a fixed port and that key in `<DOCBANK_HOME>/config.toml` (`~/.docbank/config.toml`). Without this section its daemon picks a new port and a new key each time it starts, and kata loses it after the next restart:

   ```toml
   [server]
   bind_addr = "127.0.0.1"
   api_port = 8080
   api_key = "<the key from ~/.kata/docbank.key>"
   ```

   Docbank does not document these keys in its help; they are checked against Docbank 0.17. Pick any free port; it must match the `url` in step 5.
4. Restart Docbank so it reads them: `docbank daemon restart`. `docbank daemon status` then shows `127.0.0.1:8080`.
5. Add a `[docbank]` section to `<KATA_HOME>/config.toml` (`~/.kata/config.toml` unless you moved it):

   ```toml
   [docbank]
   url = "http://127.0.0.1:8080"
   api_key_file = "/home/me/.kata/docbank.key"
   folder = "/kata"
   ```

   - Give the key as one of `api_key_file`, `api_key_env` (the name of an environment variable) or `api_key`; a file is the safest. The key is read again for each request, so you can rotate it without restarting kata (Docbank needs `docbank daemon restart` after its `api_key` changes).
   - `folder` is an absolute path in the vault and defaults to `/kata`. kata adds one folder per Project below it.
   - `KATA_DOCBANK_URL`, `KATA_DOCBANK_API_KEY_FILE` and `KATA_DOCBANK_FOLDER` set the same three in the daemon's environment.
6. Restart the kata daemon: `kata daemon restart`. The Pane, `@` and the Attachments tab work from the next read.

**Keep Docbank running.** A `docbank` command starts Docbank's daemon when it is not running, but kata's requests do not. After a reboot, or once Docbank has stopped, the Pane says kata could not reach Docbank until something starts it again: `docbank daemon start`, or `docbank daemon run` as a service your OS starts at login (a systemd user unit, a launchd agent, a Windows scheduled task).

**Telemetry.** Docbank reports usage unless its daemon's environment sets `DOCBANK_TELEMETRY_ENABLED=0`.

### Docbank on another machine

Whoever runs that Docbank gives you its URL and an API key; keeping it running is theirs. With `--url` the command points kata at it:

```sh
tern-kata docbank setup --url https://docbank.example --key-file ~/docbank.key
```

- `--key-file -` reads the key from standard input and stores it in `<KATA_HOME>/docbank.key` (mode 0600); `--key-env <NAME>` names a variable the kata daemon's own environment holds instead. No flag takes the key itself, so it never lands in your shell history or a process list.
- It first checks that the URL answers with the key and writes nothing otherwise: a refused key and an unreachable Docbank are reported in the words the Pane uses. The `url` must be HTTPS: kata accepts plain HTTP only for Docbank on this machine, and the command refuses it before writing anything.
- It then appends a `[docbank]` section (`url`, the key's source, `folder`) to `<KATA_HOME>/config.toml`, leaving every other byte of the file as it is, and creates the file when it is missing. Running it again says "already set up"; a different `[docbank]` section is refused and shown, never overwritten.
- `--folder` picks the vault folder (default `/kata`). The route flags `tern-kata agent` takes choose another kata daemon, a local profile for instance (`--daemon <name>`); a remote kata daemon is refused, because its config is not on this machine.
- It asks before it restarts the kata daemon (`kata daemon restart`, run with `KATA_HOME` set to the profile's home for a profile, since kata's daemon commands take no `--daemon`), then checks that the daemon has attachments on. `--yes` skips the question; `--no-restart`, or no terminal without `--yes`, prints the command to run instead.
- It prints the paths it wrote and the URL, never the key.

To do the same by hand:

1. Save the key in a file only you can read (`chmod 600`).
2. Add the `[docbank]` section from step 5 above with that `url` and `api_key_file`. The `url` must be HTTPS: kata accepts plain HTTP only for Docbank on this machine.
3. Restart the kata daemon.

## Moving old attachments

Before kata stored attachments, tern-kata copied each file to `.kata-attachments/<Issue ULID>/<name>` at the root of the Project's primary checkout (the main worktree; outside git, the directory holding `.kata.toml`) and kept a pointer on the Issue: one `tern-kata.attachment.<name>` meta key holding `{"host", "path"}`, the host name of the machine with the copy and its absolute path. Version 0.3 wrote the same pointer under `attachment.<name>`; the migration reads those too. The Pane no longer lists these. Move them once per machine:

```sh
tern-kata migrate-attachments [--as <actor>]
```

Run it from a directory bound to the Project. For every Issue of the Project, open and closed, and every old key whose `host` is this machine (compared without regard to case) and whose file is in that Issue's `.kata-attachments` folder, it:

1. uploads the file under the name in the key,
2. clears the old key, and only then
3. deletes the local copy.

A failure before the key is cleared keeps the file and the key, so a run can be repeated. If the upload worked but the old key could not be cleared, the command reads the Issue again: while the old key is still there it unlinks the copy it just uploaded, so a repeat does not leave two; if the key is gone (the clear reached kata though the answer was lost) the move counts as done and the local copy is deleted. If that unlink fails, or the Issue cannot be read, the output says so and names the new attachment, because a duplicate may exist; remove it with `kata docbank attachments rm`. When the `.kata-attachments` folder ends up empty (or holding only its own `.gitignore`), the command removes it.

The command prints what moved, and lists what it left with the reason:

- a key whose `host` is another machine: run the command there;
- a file that is gone from this machine;
- a file outside the Issue's folder in `.kata-attachments`, which is never deleted: add it with `kata docbank attachments add` and clear the key yourself (`kata meta unset <ref> <key>`, with `<key>` the key the list names). A file, Issue folder or store folder that is a symlink counts as outside, so the link's target is never uploaded, with the reason "outside this Project's old attachment folder";
- a `tern-kata.attachment.*` key whose value is not a `{host, path}` pointer, so it is not an old attachment;
- a file refused before upload, for its size or type, with the reason shown. The command then exits non-zero;
- an upload or key change that failed, with kata's reason. The command then exits non-zero;
- a file moved whose local copy could not be deleted: the key is already cleared, so delete the file by hand. The command then exits non-zero.

A daemon without Docbank refuses before touching anything. The command is not needed on a machine with no old attachments, and tern-kata itself no longer reads or writes the old keys.
