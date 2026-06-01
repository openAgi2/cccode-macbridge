# Backends And Configuration

MacBridge ships with local development defaults only. Real relay endpoints,
route credentials, OpenCode credentials, and signing credentials must be
configured by the user or release owner outside Git.

## Backend Requirements

- Claude Code: install and authenticate the `claude` CLI.
- OpenCode: run an OpenCode server locally. If it requires auth, configure
  username/password in MacBridge settings or private environment variables.
- Codex app-server: run the Codex app-server and point MacBridge to its
  WebSocket URL, usually `ws://localhost:4141`.
- Copilot ACP: not part of the current migrated runtime.

## Configuration Inputs

Supported configuration surfaces:

- MacBridge app settings.
- Runtime CLI flags for non-sensitive local settings such as port, backend
  mode, and local service URLs.
- Private environment variables for credentials, route identifiers, and
  management tokens.
- Private config files that are never committed.

Use `config.example.env` as a placeholder reference. It intentionally contains
no production relay endpoint, token, route ID, password, or signing identity.
MacBridge passes sensitive runtime values through the child process environment
instead of argv so they do not appear in ordinary process listings.

## Relay

No public production relay is hardcoded in this repository. Users must either:

- Run direct local WebSocket pairing on the same network.
- Self-host `relay-server` and enter that endpoint in MacBridge settings.
- Use an owner-approved hosted relay that is documented outside the public
  source tree.
