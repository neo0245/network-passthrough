# SLPP

SLPP is a lightweight Go client/server tunnel that carries multiplexed TCP and UDP traffic over a persistent TLS 1.3 + HTTP/2 connection.

## Binaries

The CLI ships as two programs:

- `slppd`: server daemon
- `slppc`: client CLI and local SOCKS5 proxy

## Documentation

- [User Manual](docs/USER_MANUAL.md)
- [Release Notes v0.1.0](docs/RELEASE_NOTES_v0.1.0.md)
- [Testing Guide](TESTING.md)

## Build

```bash
/usr/local/go/bin/go test ./...
/usr/local/go/bin/go build ./cmd/slppc
/usr/local/go/bin/go build ./cmd/slppd
```

## Release Packaging

Build the release editions with:

```bash
./scripts/build_release.sh v0.1.0
```
