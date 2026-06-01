CREATE INDEX IF NOT EXISTS idx_accounts_username_nocase ON accounts(username COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_communities_name_nocase ON communities(name COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_channels_name_nocase ON channels(name COLLATE NOCASE);
