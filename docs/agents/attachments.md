# Attachments for agents

An Issue's Attachments are `attachment.<name>` keys in its kata meta, shown by `kata show`, `kata show --json` and kata's MCP show tool. Each value is a JSON object:

```json
{"host": "<OS host name of the machine holding the file>", "path": "<absolute native path>"}
```

- The key's `<name>` is the stored file name; it always matches the file's name in the folder. Keys are flat and may contain dots and spaces.
- Open `path` with your own file or vision tools only when `host` is your machine's host name. Otherwise the file is on another machine: say so, don't retry.
- The file lives in `<primary checkout>/.kata-attachments/<Issue ULID>/<name>`, gitignored and never committed.

Add and remove with the command line, never by writing the meta key yourself:

```sh
tern-kata attach <ref> <file>...   # prints each stored absolute path
tern-kata detach <ref> <name>
```

`<ref>` is a short id in this workspace's Project, `<project>#<short id>` or a ULID; the Issue's Project must be the workspace's Binding. `--as <actor>` credits the history; the default is `$KATA_AUTHOR`, `$USER`, then `git config user.name`. A failure exits non-zero with kata's message on stderr. Reference an attached file in a comment by the exact path `attach` printed.
