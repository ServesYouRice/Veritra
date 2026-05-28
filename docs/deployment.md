# Deployment

## Single Binary

```sh
messenger-server migrate
messenger-server serve
```

Environment:

- `PRIVATE_MESSENGER_ADDR`, default `:8080`
- `PRIVATE_MESSENGER_DATA_DIR`, default `./data`
- `PRIVATE_MESSENGER_DB_PATH`, default `<data>/private-messenger.db`
- `PRIVATE_MESSENGER_STORAGE_PATH`, default `<data>/blobs`

An example environment file is available at `server/config.example.env`.

## Docker Compose

Use `deploy/docker-compose.yml` for the simple local deployment. Caddy is optional for public HTTPS.

## Network Modes

- LAN/private mode: bind to a private address and use local trust.
- Tailscale/ZeroTier mode: bind on the private interface and keep public exposure closed.
- Public VPS: run behind Caddy with automatic HTTPS.
