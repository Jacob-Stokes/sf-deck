# Safety levels

Every org sf-deck talks to has a **safety level**. The level
controls what kinds of writes sf-deck will let through. Writes
above the level are refused before they leave your machine — no
API call, no Salesforce side-effect.

## The four levels

From least to most permissive:

| Level | Allows |
|---|---|
| `read_only` | reads only — SOQL, describe, record.get, retrieve |
| `records` | record DML (`record.create / .update / .delete`) |
| `metadata` | metadata CRUD (`metadata.*`, `bundle.validate`, `bundle.deploy`) |
| `full` | destructive metadata (`metadata.delete`) and anonymous Apex — the highest tier because it can do anything |

Every verb that mutates Salesforce declares its required level in
the [verb registry](../reference/cli.md). The TUI doesn't even offer
the action when the org's level is lower; the CLI returns
`safety_blocked` with `details.required_write_kind` so a script
knows what to raise.

## Why this exists

Salesforce profile permissions are coarse and slow to change. If
your user has the System Administrator profile, you can do anything
in any org — including the production org you only meant to read
from. sf-deck's safety layer sits on top of that: a per-org override
that limits what sf-deck will do regardless of what the org would
allow.

You set production to `read_only` once. From then on, sf-deck refuses
to write to prod even when you forget which org you're in.

## Where the level lives

In `~/.sf-deck/settings.toml`, per-org. The level is set explicitly
— there's no auto-detection from org type. A sandbox can be
`read_only`; production can be `metadata` if you really want.

The header pill next to each org name in the left rail shows the
current level. Different colours per level — your visual signal
that you've just switched into prod.

## Raising and dropping

The safe pattern is **raise → do the work → drop back**.

```sh
sf-deck org safety set --org <alias> --level metadata --json
# ... validate / deploy / whatever ...
sf-deck org safety set --org <alias> --clear --json
```

`--clear` reverts to whatever the global default is (typically
`read_only`).

For production: **always ask before raising**. sf-deck won't do this
for you — that's the agent skill's job, and your own discipline when
working manually.

## What happens when you try a write the level disallows

The verb returns:

```json
{
  "ok": false,
  "error": {
    "code": "safety_blocked",
    "message": "...",
    "details": {
      "required_write_kind": "metadata",
      "effective_safety": "read_only",
      "target": "<org-alias>"
    }
  }
}
```

You haven't burned an API call. The gate fires locally before
sf-deck shells out. Raise + retry.

## Layered with profile perms

The safety level only constrains; it doesn't grant. If your user
doesn't have permission to update Account.Phone in the org,
`record.update` fails with a Salesforce-side error regardless of
your safety level being `records`.

Think of safety as "sf-deck's permission to attempt the action."
Salesforce still gets the final say.

## Related

- [Tasks → Bundle and deploy](../tasks/bundle-and-deploy.md) — the
  recipe that exercises this most.
- [Agent integration → Safety](../agent-integration/safety.md) —
  how agents are expected to handle this.
