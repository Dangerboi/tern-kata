# Attachments

Attach a screenshot, log or any other file to an Issue so whoever picks it up sees what you saw.

![The Attachments tab of "Add Apple Pay to checkout" listing a screenshot and a log](../images/attachments.png)

## In a Pane

- `@` on the selected Issue (in the list, on the Board, in the Lane view or in the Issue detail) asks for a path, quoted, starting with `~` or relative to the directory, and attaches the file. A refusal or a failure shows in the form.
- `U` lists the Issue's Attachments by name: `down` and `up` move and `enter` removes the one chosen, with no further confirmation, and a notice says what happened to the file. `U` on an Issue with none says so.
- Both keys explain why when this directory is not bound to the Issue's Project.

The Attachments tab of the Issue detail lists the Issue's `tern-kata.attachment.*` meta keys by name: a file held on this machine is a link to it, and one held on another machine reads `<name> on <host>`.

## From a shell or for an agent

```sh
tern-kata attach <ref> <file>...   # prints each stored absolute path, one per line
tern-kata detach <ref> <name>
```

- `<ref>` is anything `kata show` takes: a short id in this directory's Project, `<project>#<short id>`, or a ULID.
- `--as <actor>` names the actor; without it the actor is `$KATA_AUTHOR`, `$USER`, then `git config user.name` (an empty variable counts as unset). An explicitly empty value (`--as=` or `--as ""`) is refused with "--as must be non-empty" and nothing is written, so a script that expands an unset variable into `--as` never acts as someone else.
- Both commands run without Tern and never open a Pane; a failure prints kata's message on stderr and exits non-zero.
- `attach` stops at the first failing file and keeps those already attached.
- The Issue's Project must be the Project this directory is bound to, and a closed Issue is fine.
- The file is copied, so deleting the original breaks nothing, and a path in quotes, starting with `~` or relative to the directory all work.

## Where the copy goes

The copy goes to `.kata-attachments/<Issue ULID>/<name>` at the root of the Project's primary checkout (the main worktree, so linked worktrees share it; outside git, the directory holding `.kata.toml`). The folder carries its own `.gitignore` with `*`, so nothing shows in `git status` and your repo's ignore files stay untouched. A name already used on the Issue becomes `name-2.ext`, then `name-3.ext`.

## The pointer agents read

The pointer agents read is one kata meta key per Attachment, visible in `kata show`, `kata show --json` and kata's MCP show tool:

```json
{"tern-kata.attachment.shot.png": {"host": "my-laptop", "path": "/home/me/repo/.kata-attachments/01J.../shot.png"}}
```

`host` is the OS host name of the machine that holds the copy and `path` is the absolute native path there.

The key has its own prefix because kata's native attachments use `attachment.<id>` with a different value. Pointers written by tern-kata 0.3 sit under `attachment.<name>` with just `host` and `path`; tern-kata moves them to the new key by itself, once per Project and kata daemon on each machine, when a Pane for the Project starts or `tern-kata attach` or `detach` runs. Only a value that is exactly a `{"host", "path"}` object of strings moves, and the files stay where they are. A name already taken under the new key becomes `name-2.ext`. If some Issue can't be converted, the next start tries again.

`detach` clears the key, then deletes the copy only if `host` is this machine; otherwise it says the file was left on that host, or was already gone. If the pointer is cleared but the copy can't be deleted, `detach` still prints `Removed <name>; could not delete <path>: <reason>` and exits non-zero, and `U` shows the same line as its notice. A pointer that changed while it was being removed is left alone and nothing is deleted.

## Hosts and remote daemons

The file must already be on the session host, the machine running tern-kata. With a Windows Tern window and a Linux session host, copy a Windows file over first.

With a remote kata daemon, attaching still works for your own agents on this machine, but teammates' agents see a path on your machine that they can open only if they can SSH to it; the `tern-kata` agent skill copies the file over SSH when that works.
