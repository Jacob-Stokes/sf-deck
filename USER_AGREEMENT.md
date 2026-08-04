# User agreement

Effective: 23 July 2026

Policy version: 2026-07-23

This agreement applies when you use sf-deck to access a real Salesforce org.
The software itself remains licensed under the
[Apache License 2.0](LICENSE). This agreement does not remove rights granted by
that open-source licence or charge a fee for sf-deck.

## Your authorization

You may use sf-deck only with Salesforce orgs, data, and functionality that you
are authorized to access. You are responsible for:

- complying with the agreements and policies that apply to your Salesforce
  account, including the
  [Salesforce API terms](https://www.salesforce.com/company/legal/sfdc-api-terms-of-service/)
  and usage limits
- choosing an appropriate Salesforce user and permission set
- protecting your computer, terminal, Salesforce CLI session, exports, and
  local sf-deck data
- reviewing commands and targets before approving writes

sf-deck reuses your Salesforce CLI authorization. It does not grant additional
Salesforce rights or bypass Salesforce permissions.

## Local tool, not a hosted service

sf-deck runs on your computer and communicates directly with your selected
Salesforce instance. The maintainer does not receive, host, or control your
Salesforce customer data. See the [privacy notice](PRIVACY.md) for the exact
in-memory and on-disk behaviour.

You direct all reads, writes, exports, bundles, scripts, and agent actions
performed through sf-deck. Local safety levels are an additional guardrail, not
a substitute for Salesforce permissions, change control, backups, review, or
your own security obligations.

## Automation and agents

If you use sf-deck's headless CLI, control socket, bundled agent skill, or
another automated system, you are responsible for the automation's identity,
permissions, instructions, supervision, and output.

Automation that uses Salesforce APIs must follow the Salesforce terms that
apply to your use, including the
[Salesforce Agent Integration Protocols](https://developer.salesforce.com/salesforce-agent-integration-protocols)
when applicable. Use least privilege, keep org safety levels conservative, and
require human review for consequential or destructive actions.

## Open-source status and third parties

sf-deck is free and open source under Apache-2.0. It is not affiliated with,
endorsed by, or sponsored by Salesforce, Inc. “Salesforce” is a registered
trademark of Salesforce, Inc.

Salesforce CLI, Salesforce services, GitHub, Homebrew, operating systems, and
other third-party tools or services have their own terms and privacy policies.
The project does not control them.

## No warranty

The software is provided on the warranty and liability terms in the Apache
License 2.0. In particular, it is provided on an “AS IS” basis, without
warranties or conditions of any kind. You are responsible for deciding whether
sf-deck is suitable for a particular org or workflow.

## Ending use and changes

You can stop using sf-deck at any time, erase its local data, and disconnect
Salesforce CLI sessions using the steps in the
[privacy notice](PRIVACY.md#inspect-delete-and-revoke).

Material changes receive a new policy version and require acknowledgement
before sf-deck contacts a real org again. You can review the current status
with `sf-deck legal status`.

Questions can be sent to **hello@jacobstokes.com**.
