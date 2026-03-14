# SLPP User Manual

## 1. Overview

SLPP provides a secure tunnel between a client and a server using:

- TCP
- TLS 1.3
- HTTP/2
- SLPP binary frames

The two command-line programs are:

- `slppd`: accepts tunnel connections and relays traffic to remote targets
- `slppc`: connects to `slppd` and exposes a local SOCKS5 proxy

## 2. Quick Start

### Server

Create a token store and generate a token:

```bash
slppd gen-token --token-file tokens.json --json
```

Start the server with a TLS certificate and key:

```bash
slppd run \
  --listen 127.0.0.1:8443 \
  --cert server.crt \
  --key server.key \
  --token-file tokens.json \
  --control-socket /tmp/slppd.sock
```

### Client

Check connectivity:

```bash
slppc ping --server https://127.0.0.1:8443 --insecure
```

Start the local SOCKS5 proxy:

```bash
slppc socks5 \
  --server https://127.0.0.1:8443 \
  --token YOUR_TOKEN \
  --listen 127.0.0.1:1080 \
  --insecure
```

Applications can then use `127.0.0.1:1080` as a SOCKS5 proxy.

## 3. Commands

### `slppd`

- `run`: start the server in the foreground
- `service`: alias for `run`
- `check`: validate configuration inputs
- `gen-token`: generate an opaque bearer token
- `revoke-token`: revoke an existing token by ID
- `stats`: read runtime stats from the control socket
- `bench`: run a small codec benchmark helper
- `version`: print the version

### `slppc`

- `connect`: establish a raw tunnel session
- `socks5`: expose a local SOCKS5 listener backed by the tunnel
- `check`: validate client configuration inputs
- `ping`: call the server health endpoint
- `stats`: print a local empty stats envelope placeholder
- `version`: print the version

## 4. TLS Notes

- TLS 1.3 is required.
- HTTP/2 is negotiated using ALPN `h2`.
- For local or self-signed testing, use `--insecure` on the client.
- For production use, deploy a valid certificate and avoid `--insecure`.

## 5. Token Management

Tokens are opaque bearer tokens stored in a JSON token file.

Generate a token:

```bash
slppd gen-token --token-file tokens.json
```

Revoke a token:

```bash
slppd revoke-token --token-file tokens.json --id TOKEN_ID
```

## 6. Control Socket

The server exposes a local control API over a Unix domain socket. Use it to query runtime stats:

```bash
slppd stats --control-socket /tmp/slppd.sock --json
```

## 7. SOCKS5 Usage

SLPP supports:

- TCP `CONNECT`
- UDP `ASSOCIATE`

Typical desktop or CLI tools can route traffic through `slppc socks5` by pointing them at the local SOCKS5 listener.

## 8. Supported Release Editions

This first edition includes:

- Windows x86_64
- Windows arm64
- Linux arm64
- macOS arm64

Each edition archive contains:

- `slppc`
- `slppd`
- this user manual
- release notes

## 9. Known Limitations

- The control-plane implementation is minimal.
- The current Windows build is cross-compiled, but operational behavior should still be verified in a Windows environment before production use.
- The release is CLI-first and intended for early adopters and testing.
