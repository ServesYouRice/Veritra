CREATE TABLE IF NOT EXISTS instances (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  name TEXT NOT NULL,
  setup_complete INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  email TEXT,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'moderator', 'member')),
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL,
  deleted_at TEXT
);

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  key_package BLOB NOT NULL,
  signing_key BLOB,
  created_at TEXT NOT NULL,
  last_seen_at TEXT,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS invites (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  created_by TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  max_uses INTEGER NOT NULL DEFAULT 1,
  uses INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  created_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS communities (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS channels (
  id TEXT PRIMARY KEY,
  community_id TEXT NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('private', 'announcement')),
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('dm', 'group', 'community_channel')),
  title TEXT,
  community_id TEXT REFERENCES communities(id) ON DELETE SET NULL,
  channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL,
  created_by TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  retention_seconds INTEGER,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memberships (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  community_id TEXT REFERENCES communities(id) ON DELETE CASCADE,
  conversation_id TEXT REFERENCES conversations(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'moderator', 'member')),
  created_at TEXT NOT NULL,
  UNIQUE(account_id, community_id),
  UNIQUE(account_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS message_envelopes (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  sender_account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  sender_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL,
  ciphertext BLOB NOT NULL,
  crypto_protocol TEXT NOT NULL,
  crypto_metadata_json TEXT NOT NULL,
  attachment_refs_json TEXT NOT NULL,
  reply_to_id TEXT REFERENCES message_envelopes(id) ON DELETE SET NULL,
  thread_root_id TEXT REFERENCES message_envelopes(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  edited_at TEXT,
  deleted_at TEXT,
  expires_at TEXT,
  CHECK (length(ciphertext) > 0),
  UNIQUE(sender_device_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS attachment_envelopes (
  id TEXT PRIMARY KEY,
  owner_account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  conversation_id TEXT REFERENCES conversations(id) ON DELETE CASCADE,
  storage_key TEXT NOT NULL UNIQUE,
  ciphertext_sha256 TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  crypto_metadata_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS reactions (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES message_envelopes(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  reaction_ciphertext BLOB NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(message_id, account_id)
);

CREATE TABLE IF NOT EXISTS read_receipts (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  message_id TEXT NOT NULL REFERENCES message_envelopes(id) ON DELETE CASCADE,
  read_at TEXT NOT NULL,
  PRIMARY KEY(account_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS disappearing_policies (
  conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
  retention_seconds INTEGER NOT NULL,
  updated_by TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS push_subscriptions (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  device_id TEXT REFERENCES devices(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  public_key TEXT,
  auth_secret TEXT,
  created_at TEXT NOT NULL,
  disabled_at TEXT
);

CREATE TABLE IF NOT EXISTS backup_blobs (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  storage_key TEXT NOT NULL UNIQUE,
  size_bytes INTEGER NOT NULL,
  key_derivation_metadata_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS call_sessions (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  created_by TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  state TEXT NOT NULL,
  metadata_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  ended_at TEXT
);

CREATE TABLE IF NOT EXISTS sync_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type TEXT NOT NULL,
  account_id TEXT,
  conversation_id TEXT,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  event_type TEXT NOT NULL,
  metadata_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_devices_account ON devices(account_id);
CREATE INDEX IF NOT EXISTS idx_sessions_account ON sessions(account_id);
CREATE INDEX IF NOT EXISTS idx_memberships_account ON memberships(account_id);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON message_envelopes(conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_sync_events_account ON sync_events(account_id, id);

