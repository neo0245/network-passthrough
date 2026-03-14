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

TLS certificate configuration for the server is currently done with:

- `--cert`: PEM certificate chain
- `--key`: PEM private key

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
- The server enforces TLS 1.3 and requires `--cert` and `--key`.
- The client also uses TLS 1.3.
- The current client implementation verifies certificates by default through the system trust store.
- For local or self-signed testing, use `--insecure` on the client.
- There is not yet a dedicated `--ca-file` or certificate pinning option in the CLI, so self-signed production deployment is not fully configurable from the client side today.
- For production use, deploy a certificate chain trusted by the client host and avoid `--insecure`.

### Current TLS Configuration Points

Server:

- certificate: `slppd run --cert /path/to/server.crt`
- private key: `slppd run --key /path/to/server.key`

Client:

- server URL: `slppc ... --server https://host:port`
- testing override: `slppc ... --insecure`

### Verification Requirement

If you require strict verification on both sides:

- server identity is already verified by the client when `--insecure` is not used and the certificate chains to a trusted CA
- server-side mutual TLS client-certificate verification is not implemented in this release
- client-side custom CA configuration is not implemented in this release

That means this edition guarantees TLS 1.3 transport, but full operator-configurable mutual certificate verification is still a future enhancement.

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

## 7. Linux systemd Service

A sample unit file is provided at:

- `deploy/systemd/slppd.service`

Suggested install steps on Linux:

1. Install `slppd` to `/usr/local/bin/slppd`
2. Create `/etc/slpp/` for certificates and token store
3. Copy the unit file to `/etc/systemd/system/slppd.service`
4. Adjust paths in the unit file if needed
5. Reload systemd and enable the service

Example:

```bash
sudo install -m 0755 slppd /usr/local/bin/slppd
sudo mkdir -p /etc/slpp
sudo cp deploy/systemd/slppd.service /etc/systemd/system/slppd.service
sudo systemctl daemon-reload
sudo systemctl enable --now slppd
```

Useful commands:

```bash
sudo systemctl status slppd
sudo journalctl -u slppd -f
sudo systemctl restart slppd
```

## 8. SOCKS5 Usage

SLPP supports:

- TCP `CONNECT`
- UDP `ASSOCIATE`

Typical desktop or CLI tools can route traffic through `slppc socks5` by pointing them at the local SOCKS5 listener.

## 9. Supported Release Editions

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

## 10. Known Limitations

- The control-plane implementation is minimal.
- The current Windows build is cross-compiled, but operational behavior should still be verified in a Windows environment before production use.
- The release is CLI-first and intended for early adopters and testing.
- Mutual TLS client-certificate authentication is not implemented.
- The client does not yet expose a dedicated custom CA file option.
