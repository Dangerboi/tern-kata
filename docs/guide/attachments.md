# Attachments

Attach a screenshot, log or any other file to an Issue so whoever picks it up sees what you saw. kata stores the file: its bytes go to Docbank and the Issue keeps a reference to them, so every agent and person who can read the Issue can download the file, from any machine.

![The Attachments tab of "Add Apple Pay to checkout" listing a screenshot and a log](../images/attachments.png)

This needs kata v0.21 and a daemon with Docbank configured (see [Setting Docbank up](#setting-docbank-up)). Without it the Pane says so plainly and nothing can be attached.

## In a Pane

- `@` on the selected Issue (in the list, on the Board, in the Lane view or in the Issue detail) asks for a path, quoted, starting with `~` or relative to the directory, and uploads the file under its own name. A refusal or a failure shows in the form. A file may be up to 256 MiB.
- `U` lists the Issue's Attachments by name: `down` and `up` move and `enter` unlinks the one chosen, with no further confirmation. Unlinking removes the Issue's reference only; the bytes stay in Docbank, because kata has no call that deletes them. Two Attachments with one name show with the last characters of their ids to tell them apart. `U` on an Issue with none says so.
- The Attachments tab of the Issue detail lists the Issue's Attachments with their name, media type and size. `enter` on one downloads it into a cache folder and opens it with the program your OS has for its type; the next `enter` reuses the download.

The cache folder is `tern-kata/attachments/<Issue UID>/<attachment id>/<name>` under your user cache directory (`$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows). It is yours to delete.

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

You do this once, for the kata daemon. tern-kata cannot do it for you.

1. Install Docbank and create a vault that every reader of your Issues may open. kata puts files in a folder of that vault; the folder only organises them and is no access control, so whoever can read an Issue can download its attachments.
2. Create an API key for it and save it in a file only you can read (`chmod 600`).
3. Add a `[docbank]` section to `<KATA_HOME>/config.toml` (`~/.kata/config.toml` unless you moved it):

   ```toml
   [docbank]
   url = "http://127.0.0.1:8080"
   api_key_file = "/home/me/.kata/docbank.key"
   folder = "/kata"
   ```

   - `url` must be HTTPS unless it points to this machine.
   - Give the key as one of `api_key_file`, `api_key_env` (the name of an environment variable) or `api_key`; a file is the safest. The key is read again for each request, so you can rotate it without a restart.
   - `folder` is an absolute path in the vault and defaults to `/kata`. kata adds one folder per Project below it.
   - `KATA_DOCBANK_URL`, `KATA_DOCBANK_API_KEY_FILE` and `KATA_DOCBANK_FOLDER` set the same three in the daemon's environment.
4. Restart the kata daemon. The Pane, `@` and the Attachments tab work from the next read.

Docbank has telemetry settings of its own; check Docbank's documentation for turning them off.

## Moving old attachments

Before kata stored attachments, tern-kata copied each file to `.kata-attachments/<Issue ULID>/<name>` at the root of the Project's primary checkout (the main worktree; outside git, the directory holding `.kata.toml`) and kept a pointer on the Issue: one `tern-kata.attachment.<name>` meta key holding `{"host", "path"}`, the host name of the machine with the copy and its absolute path. The Pane no longer lists these. Move them once per machine:

```sh
tern-kata migrate-attachments [--as <actor>]
```

Run it from a directory bound to the Project. For every Issue of the Project, open and closed, and every old key whose `host` is this machine (compared without regard to case) and whose file is in that Issue's `.kata-attachments` folder, it:

1. uploads the file under the name in the key,
2. clears the old key, and only then
3. deletes the local copy.

A failure at any step keeps the file and the key, so a run can be repeated. If the upload worked but the old key could not be cleared, the next run uploads the file again, so the Issue gets a second copy of it. When the `.kata-attachments` folder ends up empty (or holding only its own `.gitignore`), the command removes it.

The command prints what moved, and lists what it left with the reason:

- a key whose `host` is another machine: run the command there;
- a file that is gone from this machine;
- a file outside the Issue's folder in `.kata-attachments`, which is never deleted: add it with `kata docbank attachments add` and clear the key yourself (`kata meta unset <ref> tern-kata.attachment.<name>`);
- an upload or key change that failed, with kata's reason. The command then exits non-zero.

A daemon without Docbank refuses before touching anything. The command is not needed on a machine with no old attachments, and tern-kata itself no longer reads or writes the old keys.
