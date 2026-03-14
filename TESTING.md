# SLPP Testing

## Automated

Run the full suite:

```bash
/usr/local/go/bin/go test ./...
```

## Manual Smoke

1. Create a temp directory and generate a self-signed cert/key.
2. Generate a token file with `slppd gen-token --token-file ...`.
3. Start `slppd run --listen 127.0.0.1:8443 --cert ... --key ... --token-file ... --control-socket ...`.
4. Verify health with `slppc ping --server https://127.0.0.1:8443 --insecure`.
5. Start `slppc socks5 --server https://127.0.0.1:8443 --token ... --listen 127.0.0.1:1080 --insecure`.
6. Verify a TCP request through the SOCKS5 listener to a local echo or HTTP target.
7. Verify a UDP request through the SOCKS5 listener to a local UDP echo target.
8. Query `slppd stats --control-socket ... --json` while traffic is flowing.
9. Revoke the token with `slppd revoke-token --token-file ... --id ...` and confirm a new client session fails.
