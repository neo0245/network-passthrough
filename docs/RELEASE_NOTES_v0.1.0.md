# SLPP v0.1.0

## First Compiled Edition

This is the first compiled edition of SLPP.

Included deliverables:

- `slppc` client CLI
- `slppd` server daemon
- TCP and UDP tunnel support
- TLS 1.3 + HTTP/2 transport
- opaque bearer-token authentication
- SOCKS5 client support
- integration and transport tests

## Release Editions

- `slpp-v0.1.0-windows-x86_64.zip`
- `slpp-v0.1.0-windows-arm64.zip`
- `slpp-v0.1.0-linux-arm64.tar.gz`
- `slpp-v0.1.0-macos-arm64.tar.gz`

## Validation

The release build is backed by:

```bash
/usr/local/go/bin/go test ./...
```
