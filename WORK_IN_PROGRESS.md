# WORK IN PROGRESS

Findings from the 2026-05-29 security/quality audit. This file tracks open
work; close items by ticking them and moving them under "Done" with the
commit SHA that landed the fix.

---

## Already landed (this session)

| # | Fix | Files |
|---|---|---|
| 1 | Username-enumeration timing — bcrypt against dummy hash when lookup fails | auth.go, api.go login |
| 2 | Conversation privilege escalation — `effectiveConversationRole` caps grant rank | api.go members, types.go RoleRank/ValidRole |
| 3 | Session token in URL — dropped query fallback; mobile uses Authorization header on WS | api.go principalFromRequest, sync_service.dart |
| 4 | `claim_token` in URL — moved to X-Veritra-Claim-Token header | api.go claim-status, api_client.dart, api_test.go |
| 5 | WebSocket origin check (browser cross-origin upgrades rejected) | realtime/websocket.go |
| 6 | WebSocket rejects unmasked client frames (RFC 6455 §5.1) | realtime/websocket.go |
| 7 | WebSocket max frame size (1 MiB cap) | realtime/websocket.go |
| 8 | WebSocket clears http.Server deadlines after Hijack | realtime/websocket.go |
| 9 | Rate limiter: trusted-proxy aware, separate 10/min auth bucket, bounded map, periodic cleanup | app/app.go, config/config.go |
| 10 | http.Server Read/Write/Idle timeouts | app/app.go |
| 11 | Security headers: X-Frame-Options, COOP/CORP, Permissions-Policy, conditional HSTS, CSP on /setup | app/app.go |
| 12 | retention_seconds bounded to [0, 10 years] | api.go (validRetention) |
| 13 | idempotency_key ≤ 128 chars, crypto_protocol ≤ 64 chars | api.go createMessageEnvelope |
| 14 | MarkRead cannot rewind read cursor | storage/sqlite.go MarkRead |

---

## Open — Tier 1: finish hardening

_All items in this tier are landed; see Done section below._

---

## Open — Tier 2: spec gaps the Plan already names

_All items in this tier are landed; see Done section below._

---

## Open — Tier 3: largest unfinished commitments from Plan.md

- [ ] **Production E2EE crypto.** OpenMLS/libsignal binding through
  `cryptoapi.ClientCrypto`. Mark `TestOnlyCryptoService` test-only.
  Biggest single item in the project.

- [ ] **QR rendering + scanning + key-continuity check** on top of the
  existing device-link API.

- [ ] **Push providers.** APNs, FCM, UnifiedPush implementations of
  `push.Provider`. Tests proving payloads carry no message text or
  sender name.

- [ ] **WebRTC media + 1:1 calls** behind `webrtc.SignalingService`.

- [ ] **Mobile encrypted-attachment upload UX** and encrypted-backup
  restore UX.

---

## Open — Tier 4: scale & ops

- [ ] **M. Single SQLite connection serializes all I/O**
  `SetMaxOpenConns(1)`. Correct for write safety, but WAL mode allows
  concurrent reads. For small instances this is fine — note for scale.

- [ ] **H. Schema migrations have no integrity check**
  `migrationApplied()` only checks version presence. Add a content
  checksum column so silent edits to applied SQL files are detected.

- [ ] **N. `Hub.Publish` drops events on full client buffer**
  Recovery via DB-backed `/sync/events` exists. Document the contract.

---

## Done

### 2026-05-29 — Tier 2 (spec gaps)
- **L. Login device attribution.** When no `device_id` is provided,
  `LoginRecord` now picks the most-recently-active device
  (`COALESCE(last_seen_at, created_at) DESC`) instead of the oldest.
  Combined with the `last_seen_at` stamping from Tier 1, login attaches
  to the device a user actually used last.
- **Q. Settings: hidden non-functional push toggle.** Replaced the dead
  `SwitchListTile` with explicit "coming soon" disabled tiles for
  Recovery / Calls / Privacy so the UI no longer implies features that
  don't exist.
- **R. `ListMessages` cursor pagination.** Added
  `storage.ListMessagesOptions` (`Limit`, `BeforeID`, `AfterID`).
  `GET /api/v1/conversations/{id}/messages` accepts `before`/`after`
  query params and returns `next_before` when more older messages may
  exist. Cursor is `(created_at, id)` so messages with identical
  timestamps still page correctly. New `TestListMessagesCursorPagination`
  walks three pages to verify the contract.
- **S. `ExportAccount` paginated.** New `ExportAccountOptions` (Limit,
  BeforeID), default 1000 with a cap of 5000. Endpoint surfaces
  `next_before` when more messages exist so truncation is no longer
  silent. Account, devices, and conversations are still returned in full
  on every page; only messages paginate.
- **P. Mobile insecure-URL confirmation.** `ConnectScreen._submit` now
  shows a confirmation dialog when the user submits an `http://` URL
  whose host is not local (`localhost`, `127.0.0.1`, `::1`, `*.local`,
  `*.localhost`, RFC 1918 ranges). Cancel aborts the submission;
  Continue Anyway proceeds.
- **O. Encrypted local session store.** Added `flutter_secure_storage`
  dependency and `SecureLocalStore` that persists the session as JSON to
  the platform keystore (Android Keystore-backed EncryptedSharedPrefs,
  iOS Keychain `first_unlock_this_device`). `main.dart` uses it by
  default and calls `AppState.tryRestoreSession()` on cold start so the
  user is no longer kicked out on every app launch. Tests continue to
  use `MemoryLocalStore`.

### 2026-05-29 — Tier 1 hardening pass
- **K. Bearer prefix case-insensitive.** `principalFromRequest` uses
  `EqualFold` on the scheme so `bearer`, `BEARER` work per RFC 7235.
- **I. Empty community/channel names rejected.** New `validDisplayName`
  helper applied in `createCommunity` and the channel branch of
  `communitySubroute`. Returns `invalid_name`.
- **J. Invite expiry validated.** `createInvite` rejects past
  `expires_at`, caps at 90 days, and rejects `max_uses` outside [0, 10000].
- **F. Push de-registration.** `DELETE /api/v1/push/subscriptions/{id}`
  calls `Store.DisablePushSubscription`, which sets `disabled_at` only on
  rows owned by the caller. Returns 404 if not found.
- **E. `devices.last_seen_at` stamped from `PrincipalByTokenHash`.** Best-
  effort UPDATE throttled by a WHERE clause to one write per minute per
  device — no row touched if `last_seen_at` was updated within the last
  60s, so we don't write-amplify on chatty clients.
- **G. Audit events wired.** New `Store.RecordAuditEvent` writes metadata-
  only rows to the previously-unused `audit_events` table. Currently
  fired on: `owner.created`, `account.registered`, `session.login`,
  `invite.created`, `device_link.approved`,
  `conversation.member.added`, `conversation.retention.updated`,
  `account.deleted`. Payloads carry IDs and role names only — never
  ciphertext, message content, or password hashes.
- **D. `sync_events.payload_json` no longer duplicates ciphertext.**
  `messageEventRef` returns `{message_id, conversation_id, edited_at?,
  deleted_at?}` instead of the full envelope. Realtime WebSocket payloads
  still carry the full envelope so connected clients get it without a
  round trip; the persisted log is now a compact reference and the
  recovering client refetches via `/api/v1/conversations/.../messages`.
- **C. `sync_events`/`audit_events` retention sweep.** New goroutine
  `runRetentionSweeper` ticks every 6h, deleting rows older than 30 days
  (override: `PRIVATE_MESSENGER_SYNC_EVENT_RETENTION_DAYS`). Sweep also
  runs once at startup. `Store.PruneSyncEvents` and `PruneAuditEvents`
  expose the operation.
- **A. Atomic backup via `VACUUM INTO`.** `Store.BackupTo` issues
  `VACUUM INTO '<dest>'` so the snapshot includes the WAL frames and is a
  single consistent file. CLI `backup` opens the store, calls it, then
  `chmod 0600`. Refuses to overwrite an existing destination.
- **B. Safer restore.** CLI `restore` probes the `-wal` companion for
  exclusive open before touching anything; if the file is in use, refuses
  with a clear error. Removes any leftover `-wal`/`-shm` companions
  before copying the backup over the live DB so SQLite cannot replay a
  stale journal against the restored file.
