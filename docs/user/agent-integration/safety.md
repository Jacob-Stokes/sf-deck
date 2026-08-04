# Safety from an agent's perspective

The [safety gate](../concepts/safety.md) sits in sf-deck. The
agent's job is to handle it correctly.

## The shape of the decision

Before any write, an agent should answer four questions:

1. **What level does this verb require?** Check the verb's
   `safety` field in the registry.
2. **What's the org's current effective level?** `org.safety.get`.
3. **Is the effective level high enough?** If yes, fire. If no,
   go to 4.
4. **Is the org production?** Consult the user. ALWAYS ask before
   raising production safety. For pre-authorised sandboxes, raise
   without ceremony.

## The recipe

```sh
# 1. Check current
sf-deck org safety get --org <alias> --json

# 2. Raise to the required level (with confirmation for prod)
sf-deck org safety set --org <alias> --level <required> --json

# 3. Do the work
sf-deck <verb> --org <alias> --json

# 4. Drop back
sf-deck org safety set --org <alias> --clear --json
```

`--clear` reverts the override to whatever the global default is
(typically `read_only`). Always drop after the work, even if the
work failed — don't leave the org at an elevated level.

## What `safety_blocked` looks like

```json
{
  "ok": false,
  "command": "record.update",
  "error": {
    "code": "safety_blocked",
    "message": "...",
    "details": {
      "required_write_kind": "records",
      "effective_safety": "read_only",
      "target": "<alias>"
    }
  }
}
```

`required_write_kind` tells you exactly which level to raise to.
Don't jump higher than necessary — narrower is safer.

## Production rules

The skill bundles a default rule: ALWAYS confirm with a human
before raising safety on a production org. The agent's memory /
configuration tells it which orgs are production. If memory is
silent, treat every org as production and ask.

For destructive ops (`metadata.delete`, bulk record deletes), the
agent should confirm even on sandboxes if the action affects
shared / customer-relevant data.

## Why not just keep safety high

Two reasons:

1. **A safety override is sticky.** Forgetting to drop back leaves
   the org open to a later mistake.
2. **The signal matters.** When the agent (or the user) sees the
   pill flip to green, it's a real moment of "wait, am I sure I
   want this." Keeping prod always-elevated defeats that.

The raise → do → drop pattern is the contract.

## Anonymous Apex is special

Anonymous Apex requires `full`, the highest tier, because it can do
anything the running user could do — DML records, deploy metadata,
or call out. There is no separate `anonymous` safety level.

Always ask before running anonymous Apex on production. Always.
Even if the user requested it.

## Layered with profile perms

The safety gate constrains; it doesn't grant. If the user's
profile doesn't allow a write, raising sf-deck's safety doesn't
help — the write still fails at the Salesforce layer with a
permission error.

Think of sf-deck's safety as "this tool's permission to attempt
the action." Salesforce gets the final say.

## Reading current safety for diagnostics

```sh
sf-deck org safety get --org <alias> --json | jq
```

Returns:

```json
{
  "ok": true,
  "data": {
    "org_user": "...",
    "safety": "metadata",
    "override": "metadata",
    "explicit": true,
    "source": "override"
  }
}
```

- `safety` — the effective level
- `override` — what's explicitly set (empty if the level comes
  from a default)
- `explicit` — whether the user has set this per-org
- `source` — `"override"` or `"default"`

`source: "default"` means the org will revert to it on `--clear`.
`source: "override"` means there's a per-org entry that
`--clear` removes.
