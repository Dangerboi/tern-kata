---
name: tern-kata
description: Files attached to kata Issues through tern-kata. Use when an Issue's metadata has `attachment.*` keys, or to attach a screenshot, log or other file to an Issue.
---

An Attachment is a file copied into the Project's Attachment store, `<primary checkout>/.kata-attachments/<Issue ULID>/<name>`, and pointed to by one `attachment.<name>` meta key on the Issue. The store is gitignored and never committed.

## Read

Reading needs kata and your own file tools, plus SSH for a file on another host.

- `kata show <ref> --json` holds each Attachment at `.issue.metadata["attachment.<name>"]` as a `{"host", "path"}` object; `kata show --agent` and kata's MCP show tool carry the same key. `<name>` is the stored file name.
- When `host` matches `hostname` (any case), read `path` directly with your file or image tools. File searches skip the gitignored store: take the path from the pointer.
- Another host means the file is on that machine, and `host` is its OS host name. A `Host <host>` entry in `~/.ssh/config` can supply the `HostName` and `User` it needs. When `ssh -o BatchMode=yes -o ConnectTimeout=5 <host> exit` succeeds (`exit`, since a Windows host runs PowerShell or cmd, where `true` fails), copy the file to a temporary file outside the repo with `scp -o BatchMode=yes '<host>:<path>' <temp file>` and read the copy. Write a Windows `path` with `/` for each `\` (`C:/Users/...`); scp refuses the backslashes. Only read on that host; the file there stays as it is. When SSH fails (no key, no user, or a host key not yet trusted), tell the user the file is on `host` and that a `Host <host>` entry with its `User`, plus one connection to trust its key, would let you read it; then work without it.

## Attach and detach

These need the `tern-kata` binary installed and on `PATH`: check with `command -v tern-kata` (PowerShell: `Get-Command tern-kata`). When it is missing, tell the user the Attachment was left out and point them to https://github.com/Dangerboi/tern-kata#install.

```bash
tern-kata attach <ref> <file>... [--as <actor>]   # copies each file, prints its stored absolute path, one per line
tern-kata detach <ref> <name> [--as <actor>]      # clears the pointer, deletes the copy when it is on this machine, says what became of the file
```

- Attach the evidence behind a bug report, a finding or a close, then cite the printed path in the comment or close message.
- Run from a checkout of the Issue's own Project; linked worktrees share the primary checkout's store. Anywhere else is refused, and the message names the Project to work from. Closed Issues take attachments.
- `<ref>` is a short id in this checkout's Project, `<project>#<short id>`, or a ULID. `--as` defaults to `$KATA_AUTHOR`, `$USER`, then `git config user.name`.
- A name the Issue already uses is stored as `<stem>-2<ext>`, then `-3`: cite the printed path, not the original name. `attach` stops at the first failing file and keeps those already attached; any failure exits non-zero with the reason on stderr.
- The file must be on this machine. With a Windows Tern window and a Linux session host, a file on the Windows side has to be copied over first.
- Change attachments only through these two commands, which keep the copy and its pointer together (`kata meta unset` would strand the copy).
