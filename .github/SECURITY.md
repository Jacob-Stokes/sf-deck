# Security policy

## Reporting a vulnerability

Email **hello@jacobstokes.com** with the details. Please don't
open a public issue for security reports.

What to include:

- A description of the vulnerability
- Steps to reproduce (ideally on the `--demo` mode if applicable)
- The version (`sf-deck --version`)
- Your assessment of severity

I'll respond within 72 hours acknowledging receipt, and provide a
timeline for the fix in the same reply.

## Threat model

sf-deck's threat model is narrow because the surface is small:

- **No telemetry or analytics.** Normal data-plane traffic goes to the
  Salesforce instance selected through the `sf` CLI. User-invoked browser
  actions may also open documented links in the user's browser. Release builds
  make at most one anonymous, version-free GitHub Releases request every 24
  hours when automatic update checks are enabled. The check is optional,
  cached, and never downloads or installs software.
- **Credentials remain owned by the `sf` CLI.** sf-deck does not persist
  credentials itself, but it does request the current access token from `sf`,
  holds it in process memory, and uses it for direct Salesforce REST/SOAP
  calls. Treat a running sf-deck process like any other authenticated client.
- **Application state lives under `~/.sf-deck/`.** User-requested exports and
  bundles are written to the destination the user selects, and ephemeral
  demo/no-cache data may use an OS temporary directory.
- **No code execution on the user's behalf beyond what they
  invoke.** Anonymous Apex requires explicit safety raise and
  confirmation; metadata deploys require their own safety raise.
- **The IPC socket** at `~/.sf-deck/control-N.sock` is bound to the
  user's home directory with explicit user-only filesystem permissions.
  There is no remote control listener. The opt-in `SF_DECK_PPROF` diagnostics
  server is restricted to a loopback address and requires a per-process
  random access token (printed at startup).

Concretely in scope:

- Privilege escalation: a malicious local process can connect to
  the IPC socket and drive the running sf-deck instance, which
  inherits the user's Salesforce session. The socket relies on
  filesystem permissions for isolation.
- Cache integrity: `~/.sf-deck/cache.db` holds local memoised
  Salesforce data. Tampering with it could cause sf-deck to render
  stale or fabricated information. Treat the cache as the user's
  own data.
- The safety gate is local to sf-deck. It constrains what sf-deck
  will attempt; Salesforce still enforces its own permissions.

## Out of scope

- Salesforce-side vulnerabilities. Report those directly to
  Salesforce.
- Issues that require physical access to the user's machine.
- Issues in dependencies (Go stdlib, the `sf` CLI). Report
  upstream; sf-deck will pick up patched versions on the next
  build.

## Disclosure

I'll coordinate a disclosure timeline with you when a real
vulnerability is reported. The default is to publish a fix +
advisory within 30 days of the report, with credit if you want it.
