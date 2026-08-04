# Privacy and local data

sf-deck runs on your computer. It has no hosted backend, account system,
analytics, advertising, or application telemetry. The maintainer does not
receive your Salesforce org data or credentials.

Salesforce record payloads stay in process memory and are not written to the
persistent cache. That includes record lists and details, SOQL/report rows,
list-view results, and related-record lookups.

Metadata/schema caches and user-authored working state can be stored locally
under `~/.sf-deck/`. Saved SOQL text and history are local; the rows returned by
those queries are not persisted.

Read the complete [privacy notice](https://sfdeck.dev/privacy.html) for the
exact data flow, optional update request, deletion steps, and contact details.

## Inspect and erase

```sh
sf-deck data inspect
sf-deck data erase --yes
```

Close all running sf-deck processes before erasing. Add
`--include-bundles` to delete the default `~/sf-deck-bundles/` directory too.
Exports and bundles at custom paths must be removed manually.

## Disconnect an org

```sh
sf-deck org logout --org <alias-or-username> --yes
```

This removes the local Salesforce CLI authorization. A Salesforce
administrator can revoke sessions or connected-app access in Salesforce when
stronger revocation is required.
