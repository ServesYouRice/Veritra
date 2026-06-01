CREATE INDEX IF NOT EXISTS idx_sync_events_conversation ON sync_events(conversation_id, id);
CREATE INDEX IF NOT EXISTS idx_memberships_account_conversation ON memberships(account_id, conversation_id);
