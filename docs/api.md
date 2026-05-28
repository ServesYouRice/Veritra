# API

Base path: `/api/v1`

## Setup

- `GET /setup` serves the local setup UI.
- `GET /api/v1/setup/status` returns whether setup is required.
- `POST /api/v1/setup/owner` creates the first owner account and device.

## Auth and Invites

- `POST /api/v1/auth/login` returns a bearer token.
- `POST /api/v1/register` consumes an invite and creates account, device, and session.
- `POST /api/v1/invites` creates invite codes for owner/admin users.

## Messaging

- `POST /api/v1/conversations` creates DMs, groups, or channel-backed conversations.
- `GET /api/v1/conversations` lists visible conversations.
- `POST /api/v1/conversations/{id}/members` adds members.
- `PUT /api/v1/conversations/{id}/retention` updates disappearing-message retention metadata.
- `POST /api/v1/messages/envelopes` stores ciphertext-only message envelopes.
- `GET /api/v1/conversations/{id}/messages` lists encrypted envelopes.
- `POST /api/v1/messages/{id}/edit` stores an encrypted edit marker/envelope.
- `POST /api/v1/messages/{id}/delete` stores an encrypted delete marker and tombstones the server-held envelope.
- `POST /api/v1/messages/{id}/reactions` stores encrypted reaction payloads.
- `POST /api/v1/conversations/{id}/read-receipts` stores read receipt metadata.

Payloads must not include plaintext body fields. Message ciphertext is base64-encoded in JSON.

## Attachments, Backups, and Calls

- `POST /api/v1/attachments?conversation_id={id}` accepts encrypted blobs only when `X-Private-Messenger-Encrypted: 1` is present.
- `POST /api/v1/backups` accepts client-encrypted backup blobs with `X-Key-Derivation-Metadata`.
- `POST /api/v1/push/subscriptions` records push endpoints. Push payloads must remain generic.
- `POST /api/v1/calls` creates self-hosted call signaling sessions for conversation members.

Attachment and backup contents are opaque ciphertext to the server.

## Search and Account Data

- `GET /api/v1/search/metadata?q={query}` searches only account usernames, visible community names, and visible channel names.
- `GET /api/v1/account/export` exports account metadata, devices, visible conversations, and encrypted message envelopes.
- `DELETE /api/v1/account` soft-deletes the account, revokes devices, and removes sessions.

Server-side message-content search is intentionally absent.

## Realtime

WebSocket endpoint: `/api/v1/sync/ws`

Clients authenticate with `Authorization: Bearer <token>` or `?token=<token>` during development. Events are versioned and JSON encoded.

Catch-up endpoint: `GET /api/v1/sync/events?after={event_id}` returns durable sync events visible to the authenticated account.
