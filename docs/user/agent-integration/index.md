# Agent integration

sf-deck is built to be driven by AI agents as well as humans. Its two
automation transports share a backend and safety contract, while retaining
a few deliberate surface-specific verbs.

## CLI

For one-shot operations. Cold start, runs the command, exits.

```sh
sf-deck soql run --org my-sandbox --query "SELECT Id, Name FROM Account LIMIT 5" --json
```

Useful when:

- There's no sf-deck window open.
- The operation is a single read or write.
- You don't need state to persist between calls (org context, drilldowns).

## IPC

For driving a live sf-deck TUI window. One open Unix socket per
running instance.

```sh
echo '{"command":"tab.open","args":{"tab":"records","sobject":"Account"}}' \
  | nc -U ~/.sf-deck/control-1.sock
```

Useful when:

- A human is sitting in front of an sf-deck window.
- The agent wants to drive nav (`tab.open`, `chip.apply`) so the
  user sees what's happening.
- A multi-step flow benefits from the TUI rendering state between
  steps.

## Same backend, same safety, same JSON

Both transports hit the same Backend interface in Go. Both gate
writes through the same safety check. Both return the same JSON
envelope:

**Success**

```json
{"ok": true, "command": "noun.verb", "data": {...}, "changed": true}
```

**Failure**

```json
{"ok": false, "command": "noun.verb", "error": {"code": "...", "message": "..."}}
```

Exit codes (CLI) mirror the error code so scripts can branch on
either.

## The bundled skill

`skills/sf-deck/` in the repo is a Claude-skill-compatible package
that briefs an AI agent on how to drive sf-deck. It tells the
agent to:

- Discover verbs through the [registry](discovering-verbs.md), not
  via prose lists.
- Check safety before any write.
- Ask before raising production safety.
- Parse the JSON envelope rather than scrape text output.
- Pick the right transport (CLI vs IPC) per task.

Drop it into your skills directory or read it as a reference for
how to design your own agent prompt.

## Where to go next

- **[Discovering verbs](discovering-verbs.md)** — how an agent
  finds out what sf-deck can do.
- **[CLI vs IPC](cli-vs-ipc.md)** — picking the right transport.
- **[Safety from an agent](safety.md)** — the gate, from the
  agent's perspective.

For the full verb list with arguments and types:

- [CLI reference](../reference/cli.md)
- [IPC reference](../reference/ipc.md)
