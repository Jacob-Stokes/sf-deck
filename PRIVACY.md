# Privacy notice

Effective: 23 July 2026

Policy version: 2026-07-23

sf-deck is a free, open-source application maintained by Jacob Stokes. It runs
on your computer. There is no sf-deck account, hosted application backend,
analytics service, advertising system, or application telemetry.

## What the maintainer receives

The sf-deck application does not send the maintainer:

- Salesforce records, metadata, query or report results
- Salesforce credentials, access tokens, usernames, or org identifiers
- usage analytics, crash reports, machine identifiers, or profiling data

The maintainer therefore does not host, sell, share, profile, or otherwise
control your Salesforce customer data.

If you email the project or interact through GitHub, the information you choose
to provide is received through that service and used to respond to you,
maintain the project, and address security or support questions.

## How Salesforce data is processed

sf-deck uses Salesforce orgs already authenticated through Salesforce CLI. It
may request the current access token from the CLI, hold that token in process
memory, and use it for direct Salesforce REST or SOAP requests. sf-deck does
not write the token to its own databases or logs.

Salesforce record payloads are not persistently cached. This includes record
lists and details, SOQL and report result rows, list-view results, and related
record lookups. They are held in process memory while needed and disappear when
the process ends.

## Local data

sf-deck stores working state on your computer under `~/.sf-deck/`. Depending on
the features you use, this can include:

- settings, per-org safety levels, chips, and recently visited item references
- authenticated-org catalogue details and metadata/schema caches
- saved SOQL text and query history, but not returned query rows
- saved anonymous Apex text and execution history
- dev projects, tags, bundle registrations, usage counters, and application
  logs
- the cached result of the optional update check

User-requested exports and SFDX bundles are written to paths you choose.
The default bundle path is `~/sf-deck-bundles/`. Custom paths cannot be
discovered reliably by sf-deck after you choose them.

Local application files are intended to be readable only by your operating
system user. Anyone who can access your user account may be able to read them.

## Network requests

Normal data traffic goes directly from your computer to the Salesforce
instance selected through Salesforce CLI.

When enabled, the update checker makes at most one request every 24 hours to
GitHub Releases. It does not send your installed sf-deck version, Salesforce
data, credentials, or an sf-deck identifier, and it never downloads or installs
an update. GitHub may receive ordinary network information such as your IP
address under GitHub's own privacy terms. Disable automatic checks in
**Settings → Updates** or set `SF_DECK_NO_UPDATE_CHECK=1`.

Links you explicitly open may be handled by Salesforce, GitHub, your editor, or
another service under that service's own privacy terms.

## Inspect, delete, and revoke

Inspect the locations sf-deck uses:

```sh
sf-deck data inspect
```

Close every running sf-deck process, then delete sf-deck-owned application
state:

```sh
sf-deck data erase --yes
```

Add `--include-bundles` to also delete the default
`~/sf-deck-bundles/` directory. Files exported or bundled to custom paths must
be deleted from those paths manually.

Deleting sf-deck data does not remove Salesforce CLI credentials. Disconnect
an org locally with:

```sh
sf-deck org logout --org <alias-or-username> --yes
```

You can also use `sf org logout --target-org <alias-or-username>`. Salesforce
administrators can revoke sessions, connected-app access, or the user itself
through Salesforce when stronger revocation is required.

## Changes and contact

Material changes receive a new policy version and require acknowledgement
before sf-deck contacts a real org again. The current version is also shown in
**Settings → Privacy & local data** and by `sf-deck legal status`.

Questions about this notice can be sent to **hello@jacobstokes.com**. Security
issues should follow the [security policy](.github/SECURITY.md).
