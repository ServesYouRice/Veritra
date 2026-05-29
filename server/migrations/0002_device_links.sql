CREATE TABLE IF NOT EXISTS device_links (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_by_device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  state TEXT NOT NULL CHECK (state IN ('pending', 'claimed', 'approved', 'consumed', 'revoked')),
  verification_code TEXT NOT NULL,
  claimed_device_name TEXT,
  claimed_key_package BLOB,
  claimed_signing_key BLOB,
  claim_token_hash TEXT UNIQUE,
  approved_device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  claimed_at TEXT,
  approved_at TEXT,
  consumed_at TEXT,
  revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_device_links_account_state ON device_links(account_id, state);
CREATE INDEX IF NOT EXISTS idx_device_links_claim_token ON device_links(claim_token_hash);
