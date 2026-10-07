# Error codes

Every sf-deck CLI / IPC response has the same shape on failure:

```json
{
  "ok": false,
  "command": "noun.verb",
  "error": {
    "code": "...",
    "message": "...",
    "details": {}
  }
}
```

The exit code mirrors the `code` so scripts can branch on either.

| Code | Exit | Meaning |
|---|---|---|
| `invalid_argument` | 2 | Missing required arg, malformed JSON, unknown noun.verb, bad flag value |
| `not_found` | 4 | Unknown chip, tab, sObject, record, project, bundle, tag |
| `safety_blocked` | 3 | Write requires a higher safety level than the org currently has. `details.required_write_kind` tells you which level. |
| `auth_required` | 5 | Salesforce session expired. Re-auth with `sf org login web` outside sf-deck. |
| `instance_busy` | 6 | Another client holds the IPC write lock. Retry. |
| `confirmation_required` | 7 | Destructive op needs a human keystroke. `details.confirmation_token` lets you re-issue once you've accepted. |
| `method_not_implemented` | 8 | Unknown IPC command. The sf-deck instance is probably older than the verb you asked for. |
| `internal_error` | 1 | Anything else. Bug, transport error, panic. `message` carries the underlying error. |

## Common cases

### `safety_blocked`

Most common error in normal use. The write is gated; the org's
level is too low. See [Concepts → Safety](../concepts/safety.md).

The fix:

```sh
sf-deck org safety set --org <alias> --level <required> --json
# retry the original verb
sf-deck org safety set --org <alias> --clear --json
```

### `not_found` on a bundle

Bundle row exists in `~/.sf-deck/devprojects.db` but the on-disk
directory was moved or deleted. The error envelope's
`details.hint` field tells you to either re-create the directory
or unlink the bundle row (`bundle delete`).

### `instance_busy`

Two clients tried to write to the same instance at the same time.
Wait a few ms and retry. The lock is per-call, not held for the
whole session.

### `method_not_implemented` (IPC only)

You're talking to an sf-deck instance that doesn't know the verb
you asked for. Either:

- Restart the instance after upgrading the binary.
- Use the CLI instead (likely works against the same store).
- Discover what _is_ available: `sf-deck verbs list --surface ipc
  --json`.

## Reading `details`

`details` is verb-specific. Some patterns:

- **Safety**: `required_write_kind`, `effective_safety`, `target`
- **Bundles**: `bundle_id`, `path`, `hint`
- **SOQL**: `position`, `line`, `column` (when SF returns a
  MALFORMED_QUERY)
- **Metadata**: `type`, `id`, `full_name`, `salesforce_error`
- **Apex**: `compile_problem`, `line`, `column`,
  `exception_message`, `took_ms`

When in doubt, dump the response and inspect:

```sh
sf-deck some.verb --json 2>&1 | jq '.error'
```
