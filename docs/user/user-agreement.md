# User agreement

Before sf-deck discovers or contacts a real Salesforce org, it asks you to
acknowledge the current privacy notice and user agreement.

The core points are:

- use sf-deck only with orgs and data you are authorized to access
- comply with the Salesforce terms and API limits that apply to your account
- protect your computer, Salesforce CLI session, exports, and local state
- review targets and commands before writes
- supervise scripts and agents, use least privilege, and follow Salesforce's
  agent integration protocols when applicable

sf-deck remains free and open source under Apache-2.0. The acknowledgement does
not charge a fee or remove the rights granted by that licence.

Read the complete [user agreement](https://sfdeck.dev/terms.html).

Headless users can review and accept it without launching the TUI:

```sh
sf-deck legal status
sf-deck legal accept --yes
```
