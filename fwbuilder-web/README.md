# fwbuilder-web (Go) — experimental read-only MVP

This is the beginning of a **native Go port**, not yet a replacement for the C++/Qt Firewall Builder.

## What works
- Native HTTP server, no Node, Qt, Docker, or external Go modules
- Read-only import of existing Firewall Builder `.fwb` XML databases
- Object tree navigation, name/type/ID search and attribute inspector
- Policy/NAT/Routing rule lists with referenced object names
- Optional API bearer-token gate; request size and XML node limits

## Run

```sh
cd fwbuilder-web
go test ./...
go run ./cmd/fwbuilder-web -listen 127.0.0.1:8080 -file ../doc/examples.fwb
```

Browse http://127.0.0.1:8080. To run behind an HTTPS reverse proxy, set `FWBUILDER_WEB_TOKEN` (the UI prompts for it) and bind only to a trusted interface. Keep the service behind authentication; the token gate is intentionally minimal.

## Scope and safety
- No project saving, XML rewriting, compiling, install/deployment or SSH.
- Legacy XML `DOCTYPE` declarations are accepted but **external DTDs are not fetched**.
- References and attributes are displayed as data; unsupported types are preserved in the tree.
- The current parser builds an in-memory tree and index; it does not yet perform DTD/schema validation or migrations.
- Not hardened for public Internet exposure; prefer localhost with a protected reverse proxy.
- For compatible iptables/PF/Cisco compiler output, port each backend separately and compare outputs against the C++ compiler on repository fixtures.

## Proposed next steps
1. Snapshot/round-trip tests for supported `.fwb` schema versions, migrations, referential integrity.
2. Editing APIs with validation, revisions, audit trail, locking and persistence.
3. Platform-independent policy normalization and rule diagnostics.
4. iptables/nftables/PF backends, golden-file regression tests.
5. Auth/RBAC and protected deployments; compiler/install only after parity tests.
