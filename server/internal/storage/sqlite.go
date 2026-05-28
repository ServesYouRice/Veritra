package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"private-messenger/server/internal/config"
	"private-messenger/server/internal/domain"
)

var (
	ErrAlreadySetup  = errors.New("instance already has an owner")
	ErrInviteInvalid = errors.New("invite is invalid, expired, revoked, or fully used")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrNotFound      = errors.New("not found")
	ErrNotMember     = errors.New("account is not a conversation member")
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Check(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Migrate(ctx context.Context, migrations embed.FS) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		applied, err := s.migrationApplied(ctx, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, name, nowString()); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrationApplied(ctx context.Context, version string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) SetupRequired(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

type CreateOwnerInput struct {
	InstanceName string
	Username     string
	Email        *string
	PasswordHash string
	DeviceName   string
	KeyPackage   []byte
}

type AccountDevice struct {
	Account domain.Account `json:"account"`
	Device  domain.Device  `json:"device"`
}

func (s *Store) CreateOwner(ctx context.Context, input CreateOwnerInput) (AccountDevice, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountDevice{}, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&count); err != nil {
		return AccountDevice{}, err
	}
	if count > 0 {
		return AccountDevice{}, ErrAlreadySetup
	}
	accountID, err := domain.NewID("acct")
	if err != nil {
		return AccountDevice{}, err
	}
	deviceID, err := domain.NewID("dev")
	if err != nil {
		return AccountDevice{}, err
	}
	createdAt := nowString()
	instanceName := strings.TrimSpace(input.InstanceName)
	if instanceName == "" {
		instanceName = "Private Messenger"
	}
	username := domain.NormalizeUsername(input.Username)
	if _, err := tx.ExecContext(ctx, `INSERT INTO instances(id, name, setup_complete, created_at, updated_at) VALUES(1, ?, 1, ?, ?)`, instanceName, createdAt, createdAt); err != nil {
		return AccountDevice{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, username, email, password_hash, role, status, created_at) VALUES(?, ?, ?, ?, 'owner', 'active', ?)`, accountID, username, nullableString(input.Email), input.PasswordHash, createdAt); err != nil {
		return AccountDevice{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO devices(id, account_id, name, key_package, created_at) VALUES(?, ?, ?, ?, ?)`, deviceID, accountID, strings.TrimSpace(input.DeviceName), input.KeyPackage, createdAt); err != nil {
		return AccountDevice{}, err
	}
	if err := tx.Commit(); err != nil {
		return AccountDevice{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	return AccountDevice{
		Account: domain.Account{ID: accountID, Username: username, Email: input.Email, Role: domain.RoleOwner, Status: "active", CreatedAt: created},
		Device:  domain.Device{ID: deviceID, AccountID: accountID, Name: strings.TrimSpace(input.DeviceName), KeyPackage: input.KeyPackage, CreatedAt: created},
	}, nil
}

type RegisterInput struct {
	InviteCode   string
	Username     string
	Email        *string
	PasswordHash string
	DeviceName   string
	KeyPackage   []byte
}

func (s *Store) RegisterWithInvite(ctx context.Context, input RegisterInput) (AccountDevice, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountDevice{}, err
	}
	defer tx.Rollback()
	var inviteID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM invites WHERE code = ? AND revoked_at IS NULL AND uses < max_uses AND (expires_at IS NULL OR expires_at > ?)`, strings.TrimSpace(input.InviteCode), nowString()).Scan(&inviteID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountDevice{}, ErrInviteInvalid
		}
		return AccountDevice{}, err
	}
	accountID, err := domain.NewID("acct")
	if err != nil {
		return AccountDevice{}, err
	}
	deviceID, err := domain.NewID("dev")
	if err != nil {
		return AccountDevice{}, err
	}
	createdAt := nowString()
	username := domain.NormalizeUsername(input.Username)
	if _, err := tx.ExecContext(ctx, `INSERT INTO accounts(id, username, email, password_hash, role, status, created_at) VALUES(?, ?, ?, ?, 'member', 'active', ?)`, accountID, username, nullableString(input.Email), input.PasswordHash, createdAt); err != nil {
		return AccountDevice{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO devices(id, account_id, name, key_package, created_at) VALUES(?, ?, ?, ?, ?)`, deviceID, accountID, strings.TrimSpace(input.DeviceName), input.KeyPackage, createdAt); err != nil {
		return AccountDevice{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invites SET uses = uses + 1 WHERE id = ?`, inviteID); err != nil {
		return AccountDevice{}, err
	}
	if err := tx.Commit(); err != nil {
		return AccountDevice{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	return AccountDevice{
		Account: domain.Account{ID: accountID, Username: username, Email: input.Email, Role: domain.RoleMember, Status: "active", CreatedAt: created},
		Device:  domain.Device{ID: deviceID, AccountID: accountID, Name: strings.TrimSpace(input.DeviceName), KeyPackage: input.KeyPackage, CreatedAt: created},
	}, nil
}

type LoginRecord struct {
	AccountID    string
	Username     string
	PasswordHash string
	Role         string
	DeviceID     string
}

func (s *Store) LoginRecord(ctx context.Context, username, deviceID string) (LoginRecord, error) {
	username = domain.NormalizeUsername(username)
	record := LoginRecord{}
	if deviceID != "" {
		err := s.db.QueryRowContext(ctx, `
			SELECT a.id, a.username, a.password_hash, a.role, d.id
			FROM accounts a JOIN devices d ON d.account_id = a.id
			WHERE a.username = ? AND d.id = ? AND a.deleted_at IS NULL AND d.revoked_at IS NULL`, username, deviceID).
			Scan(&record.AccountID, &record.Username, &record.PasswordHash, &record.Role, &record.DeviceID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return LoginRecord{}, ErrUnauthorized
			}
			return LoginRecord{}, err
		}
		return record, nil
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.username, a.password_hash, a.role, d.id
		FROM accounts a JOIN devices d ON d.account_id = a.id
		WHERE a.username = ? AND a.deleted_at IS NULL AND d.revoked_at IS NULL
		ORDER BY d.created_at LIMIT 1`, username).
		Scan(&record.AccountID, &record.Username, &record.PasswordHash, &record.Role, &record.DeviceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoginRecord{}, ErrUnauthorized
		}
		return LoginRecord{}, err
	}
	return record, nil
}

func (s *Store) CreateSession(ctx context.Context, tokenHash, accountID, deviceID string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash, account_id, device_id, expires_at, created_at) VALUES(?, ?, ?, ?, ?)`, tokenHash, accountID, nullableEmptyString(deviceID), formatTime(expiresAt), nowString())
	return err
}

func (s *Store) PrincipalByTokenHash(ctx context.Context, tokenHash string) (domain.Principal, error) {
	principal := domain.Principal{}
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, COALESCE(s.device_id, ''), a.username, a.role
		FROM sessions s JOIN accounts a ON a.id = s.account_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND a.deleted_at IS NULL`, tokenHash, nowString()).
		Scan(&principal.AccountID, &principal.DeviceID, &principal.Username, &principal.Role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Principal{}, ErrUnauthorized
		}
		return domain.Principal{}, err
	}
	return principal, nil
}

func (s *Store) CreateInvite(ctx context.Context, createdBy string, maxUses int, expiresAt *time.Time) (domain.Invite, error) {
	if maxUses <= 0 {
		maxUses = 1
	}
	id, err := domain.NewID("inv")
	if err != nil {
		return domain.Invite{}, err
	}
	code, err := domain.NewInviteCode()
	if err != nil {
		return domain.Invite{}, err
	}
	createdAt := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO invites(id, code, created_by, max_uses, expires_at, created_at) VALUES(?, ?, ?, ?, ?, ?)`, id, code, createdBy, maxUses, nullableTime(expiresAt), formatTime(createdAt))
	if err != nil {
		return domain.Invite{}, err
	}
	return domain.Invite{ID: id, Code: code, CreatedBy: createdBy, MaxUses: maxUses, Uses: 0, ExpiresAt: expiresAt, CreatedAt: createdAt}, nil
}

func (s *Store) ListDevices(ctx context.Context, accountID string) ([]domain.Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, name, key_package, signing_key, created_at, last_seen_at, revoked_at FROM devices WHERE account_id = ? ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []domain.Device
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *Store) CreateCommunity(ctx context.Context, name, createdBy string) (domain.Community, error) {
	id, err := domain.NewID("comm")
	if err != nil {
		return domain.Community{}, err
	}
	createdAt := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Community{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO communities(id, name, created_by, created_at) VALUES(?, ?, ?, ?)`, id, strings.TrimSpace(name), createdBy, createdAt); err != nil {
		return domain.Community{}, err
	}
	membershipID, err := domain.NewID("mbr")
	if err != nil {
		return domain.Community{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memberships(id, account_id, community_id, role, created_at) VALUES(?, ?, ?, 'owner', ?)`, membershipID, createdBy, id, createdAt); err != nil {
		return domain.Community{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Community{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	return domain.Community{ID: id, Name: strings.TrimSpace(name), CreatedBy: createdBy, CreatedAt: created}, nil
}

func (s *Store) CreateChannel(ctx context.Context, communityID, name, kind, createdBy string) (domain.Channel, error) {
	role, err := s.CommunityMemberRole(ctx, communityID, createdBy)
	if err != nil {
		return domain.Channel{}, err
	}
	if !domain.CanManageMembers(role) {
		return domain.Channel{}, ErrForbidden
	}
	if kind == "" {
		kind = "private"
	}
	id, err := domain.NewID("chan")
	if err != nil {
		return domain.Channel{}, err
	}
	createdAt := nowString()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO channels(id, community_id, name, kind, created_at) VALUES(?, ?, ?, ?, ?)`, id, communityID, strings.TrimSpace(name), kind, createdAt); err != nil {
		return domain.Channel{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	return domain.Channel{ID: id, CommunityID: communityID, Name: strings.TrimSpace(name), Kind: kind, CreatedAt: created}, nil
}

type CreateConversationInput struct {
	Kind             string
	Title            *string
	CommunityID      *string
	ChannelID        *string
	CreatedBy        string
	RetentionSeconds *int64
}

func (s *Store) CreateConversation(ctx context.Context, input CreateConversationInput) (domain.Conversation, error) {
	id, err := domain.NewID("conv")
	if err != nil {
		return domain.Conversation{}, err
	}
	createdAt := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Conversation{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversations(id, kind, title, community_id, channel_id, created_by, retention_seconds, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, id, input.Kind, nullableString(input.Title), nullableString(input.CommunityID), nullableString(input.ChannelID), input.CreatedBy, nullableInt64(input.RetentionSeconds), createdAt); err != nil {
		return domain.Conversation{}, err
	}
	membershipID, err := domain.NewID("mbr")
	if err != nil {
		return domain.Conversation{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memberships(id, account_id, conversation_id, role, created_at) VALUES(?, ?, ?, 'owner', ?)`, membershipID, input.CreatedBy, id, createdAt); err != nil {
		return domain.Conversation{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Conversation{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	return domain.Conversation{ID: id, Kind: input.Kind, Title: input.Title, CommunityID: input.CommunityID, ChannelID: input.ChannelID, CreatedBy: input.CreatedBy, RetentionSeconds: input.RetentionSeconds, CreatedAt: created}, nil
}

func (s *Store) AddConversationMember(ctx context.Context, conversationID, accountID, role string) error {
	if role == "" {
		role = domain.RoleMember
	}
	id, err := domain.NewID("mbr")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO memberships(id, account_id, conversation_id, role, created_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(account_id, conversation_id) DO UPDATE SET role = excluded.role`, id, accountID, conversationID, role, nowString())
	return err
}

func (s *Store) ListConversations(ctx context.Context, accountID string) ([]domain.Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.kind, c.title, c.community_id, c.channel_id, c.created_by, c.retention_seconds, c.created_at
		FROM conversations c JOIN memberships m ON m.conversation_id = c.id
		WHERE m.account_id = ?
		ORDER BY c.created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conversations []domain.Conversation
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func (s *Store) UpdateConversationRetention(ctx context.Context, conversationID, updatedBy string, retentionSeconds *int64) (domain.Conversation, error) {
	role, err := s.ConversationMemberRole(ctx, conversationID, updatedBy)
	if err != nil {
		return domain.Conversation{}, err
	}
	if !domain.CanManageMembers(role) {
		return domain.Conversation{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Conversation{}, err
	}
	defer tx.Rollback()
	now := nowString()
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET retention_seconds = ? WHERE id = ?`, nullableInt64(retentionSeconds), conversationID); err != nil {
		return domain.Conversation{}, err
	}
	if retentionSeconds == nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM disappearing_policies WHERE conversation_id = ?`, conversationID); err != nil {
			return domain.Conversation{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO disappearing_policies(conversation_id, retention_seconds, updated_by, updated_at)
			VALUES(?, ?, ?, ?)
			ON CONFLICT(conversation_id) DO UPDATE SET retention_seconds = excluded.retention_seconds, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
			conversationID, *retentionSeconds, updatedBy, now); err != nil {
			return domain.Conversation{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Conversation{}, err
	}
	return s.ConversationByID(ctx, conversationID)
}

func (s *Store) ConversationByID(ctx context.Context, conversationID string) (domain.Conversation, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, kind, title, community_id, channel_id, created_by, retention_seconds, created_at FROM conversations WHERE id = ?`, conversationID)
	conversation, err := scanConversation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Conversation{}, ErrNotFound
		}
		return domain.Conversation{}, err
	}
	return conversation, nil
}

func (s *Store) ListConversationMemberIDs(ctx context.Context, conversationID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id FROM memberships WHERE conversation_id = ?`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ConversationMemberRole(ctx context.Context, conversationID, accountID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM memberships WHERE conversation_id = ? AND account_id = ?`, conversationID, accountID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotMember
		}
		return "", err
	}
	return role, nil
}

func (s *Store) CommunityMemberRole(ctx context.Context, communityID, accountID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM memberships WHERE community_id = ? AND account_id = ?`, communityID, accountID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotMember
		}
		return "", err
	}
	return role, nil
}

func (s *Store) IsConversationMember(ctx context.Context, conversationID, accountID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memberships WHERE conversation_id = ? AND account_id = ?`, conversationID, accountID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) SaveMessageEnvelope(ctx context.Context, envelope domain.MessageEnvelope) (domain.MessageEnvelope, bool, error) {
	existing, err := s.messageByIdempotency(ctx, envelope.SenderDeviceID, envelope.IdempotencyKey)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return domain.MessageEnvelope{}, false, err
	}
	member, err := s.IsConversationMember(ctx, envelope.ConversationID, envelope.SenderAccountID)
	if err != nil {
		return domain.MessageEnvelope{}, false, err
	}
	if !member {
		return domain.MessageEnvelope{}, false, ErrNotMember
	}
	if envelope.ID == "" {
		envelope.ID, err = domain.NewID("msg")
		if err != nil {
			return domain.MessageEnvelope{}, false, err
		}
	}
	if len(envelope.CryptoMetadata) == 0 {
		envelope.CryptoMetadata = json.RawMessage(`{}`)
	}
	if len(envelope.AttachmentRefs) == 0 {
		envelope.AttachmentRefs = json.RawMessage(`[]`)
	}
	envelope.CreatedAt = time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO message_envelopes(id, conversation_id, sender_account_id, sender_device_id, idempotency_key, ciphertext, crypto_protocol, crypto_metadata_json, attachment_refs_json, reply_to_id, thread_root_id, created_at, expires_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		envelope.ID, envelope.ConversationID, envelope.SenderAccountID, envelope.SenderDeviceID, envelope.IdempotencyKey, envelope.Ciphertext, envelope.CryptoProtocol, string(envelope.CryptoMetadata), string(envelope.AttachmentRefs), nullableString(envelope.ReplyToID), nullableString(envelope.ThreadRootID), formatTime(envelope.CreatedAt), nullableTime(envelope.ExpiresAt))
	if err != nil {
		return domain.MessageEnvelope{}, false, err
	}
	return envelope, false, nil
}

func (s *Store) messageByIdempotency(ctx context.Context, deviceID, key string) (domain.MessageEnvelope, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, conversation_id, sender_account_id, sender_device_id, idempotency_key, ciphertext, crypto_protocol, crypto_metadata_json, attachment_refs_json, reply_to_id, thread_root_id, created_at, edited_at, deleted_at, expires_at FROM message_envelopes WHERE sender_device_id = ? AND idempotency_key = ?`, deviceID, key)
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MessageEnvelope{}, ErrNotFound
		}
		return domain.MessageEnvelope{}, err
	}
	return msg, nil
}

func (s *Store) MessageByID(ctx context.Context, messageID string) (domain.MessageEnvelope, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, conversation_id, sender_account_id, sender_device_id, idempotency_key, ciphertext, crypto_protocol, crypto_metadata_json, attachment_refs_json, reply_to_id, thread_root_id, created_at, edited_at, deleted_at, expires_at FROM message_envelopes WHERE id = ?`, messageID)
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MessageEnvelope{}, ErrNotFound
		}
		return domain.MessageEnvelope{}, err
	}
	return msg, nil
}

func (s *Store) UpdateMessageEnvelope(ctx context.Context, messageID, accountID string, ciphertext []byte, cryptoProtocol string, cryptoMetadata json.RawMessage) (domain.MessageEnvelope, error) {
	if len(ciphertext) == 0 || strings.TrimSpace(cryptoProtocol) == "" {
		return domain.MessageEnvelope{}, ErrForbidden
	}
	if len(cryptoMetadata) == 0 {
		cryptoMetadata = json.RawMessage(`{}`)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE message_envelopes
		SET ciphertext = ?, crypto_protocol = ?, crypto_metadata_json = ?, edited_at = ?
		WHERE id = ? AND sender_account_id = ? AND deleted_at IS NULL`,
		ciphertext, cryptoProtocol, string(cryptoMetadata), nowString(), messageID, accountID)
	if err != nil {
		return domain.MessageEnvelope{}, err
	}
	return s.messageAfterOwnedMutation(ctx, messageID, result)
}

func (s *Store) DeleteMessageEnvelope(ctx context.Context, messageID, accountID string, markerCiphertext []byte, cryptoProtocol string, cryptoMetadata json.RawMessage) (domain.MessageEnvelope, error) {
	if len(markerCiphertext) == 0 || strings.TrimSpace(cryptoProtocol) == "" {
		return domain.MessageEnvelope{}, ErrForbidden
	}
	if len(cryptoMetadata) == 0 {
		cryptoMetadata = json.RawMessage(`{"deleted":true}`)
	}
	now := nowString()
	result, err := s.db.ExecContext(ctx, `
		UPDATE message_envelopes
		SET ciphertext = ?, crypto_protocol = ?, crypto_metadata_json = ?, deleted_at = ?
		WHERE id = ? AND sender_account_id = ? AND deleted_at IS NULL`,
		markerCiphertext, cryptoProtocol, string(cryptoMetadata), now, messageID, accountID)
	if err != nil {
		return domain.MessageEnvelope{}, err
	}
	return s.messageAfterOwnedMutation(ctx, messageID, result)
}

func (s *Store) messageAfterOwnedMutation(ctx context.Context, messageID string, result sql.Result) (domain.MessageEnvelope, error) {
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.MessageEnvelope{}, err
	}
	if rows == 0 {
		if _, err := s.MessageByID(ctx, messageID); errors.Is(err, ErrNotFound) {
			return domain.MessageEnvelope{}, ErrNotFound
		} else if err != nil {
			return domain.MessageEnvelope{}, err
		}
		return domain.MessageEnvelope{}, ErrForbidden
	}
	return s.MessageByID(ctx, messageID)
}

func (s *Store) ListMessages(ctx context.Context, conversationID, accountID string, limit int) ([]domain.MessageEnvelope, error) {
	member, err := s.IsConversationMember(ctx, conversationID, accountID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, ErrNotMember
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, conversation_id, sender_account_id, sender_device_id, idempotency_key, ciphertext, crypto_protocol, crypto_metadata_json, attachment_refs_json, reply_to_id, thread_root_id, created_at, edited_at, deleted_at, expires_at FROM message_envelopes WHERE conversation_id = ? ORDER BY created_at DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []domain.MessageEnvelope
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *Store) SaveSyncEvent(ctx context.Context, eventType string, accountID *string, conversationID string, payload interface{}) (int64, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO sync_events(event_type, account_id, conversation_id, payload_json, created_at) VALUES(?, ?, ?, ?, ?)`, eventType, nullableString(accountID), nullableEmptyString(conversationID), string(payloadBytes), nowString())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListSyncEvents(ctx context.Context, accountID string, afterID int64, limit int) ([]domain.SyncEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_type, account_id, conversation_id, payload_json, created_at
		FROM sync_events
		WHERE id > ?
		  AND (
		    account_id = ?
		    OR conversation_id IN (SELECT conversation_id FROM memberships WHERE account_id = ? AND conversation_id IS NOT NULL)
		  )
		ORDER BY id ASC
		LIMIT ?`, afterID, accountID, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.SyncEvent
	for rows.Next() {
		event, err := scanSyncEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) SearchMetadata(ctx context.Context, accountID, query string, limit int) ([]domain.MetadataSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.MetadataSearchResult{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pattern := "%" + escapeLike(query) + "%"
	results := make([]domain.MetadataSearchResult, 0, limit)
	if err := s.appendAccountSearch(ctx, &results, pattern, limit); err != nil {
		return nil, err
	}
	if len(results) >= limit {
		return results[:limit], nil
	}
	if err := s.appendCommunitySearch(ctx, &results, accountID, pattern, limit-len(results)); err != nil {
		return nil, err
	}
	if len(results) >= limit {
		return results[:limit], nil
	}
	if err := s.appendChannelSearch(ctx, &results, accountID, pattern, limit-len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Store) appendAccountSearch(ctx context.Context, results *[]domain.MetadataSearchResult, pattern string, limit int) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username FROM accounts WHERE deleted_at IS NULL AND username LIKE ? ESCAPE '\' ORDER BY username LIMIT ?`, pattern, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, username string
		if err := rows.Scan(&id, &username); err != nil {
			return err
		}
		*results = append(*results, domain.MetadataSearchResult{Type: "account", ID: id, Label: username})
	}
	return rows.Err()
}

func (s *Store) appendCommunitySearch(ctx context.Context, results *[]domain.MetadataSearchResult, accountID, pattern string, limit int) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.name
		FROM communities c JOIN memberships m ON m.community_id = c.id
		WHERE m.account_id = ? AND c.name LIKE ? ESCAPE '\'
		ORDER BY c.name
		LIMIT ?`, accountID, pattern, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		*results = append(*results, domain.MetadataSearchResult{Type: "community", ID: id, Label: name})
	}
	return rows.Err()
}

func (s *Store) appendChannelSearch(ctx context.Context, results *[]domain.MetadataSearchResult, accountID, pattern string, limit int) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ch.id, ch.name
		FROM channels ch
		JOIN memberships m ON m.community_id = ch.community_id
		WHERE m.account_id = ? AND ch.name LIKE ? ESCAPE '\'
		ORDER BY ch.name
		LIMIT ?`, accountID, pattern, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		*results = append(*results, domain.MetadataSearchResult{Type: "channel", ID: id, Label: name})
	}
	return rows.Err()
}

func (s *Store) CreateAttachmentEnvelope(ctx context.Context, attachment domain.AttachmentEnvelope) (domain.AttachmentEnvelope, error) {
	if attachment.ConversationID != nil {
		member, err := s.IsConversationMember(ctx, *attachment.ConversationID, attachment.OwnerAccountID)
		if err != nil {
			return domain.AttachmentEnvelope{}, err
		}
		if !member {
			return domain.AttachmentEnvelope{}, ErrNotMember
		}
	}
	if attachment.ID == "" {
		id, err := domain.NewID("att")
		if err != nil {
			return domain.AttachmentEnvelope{}, err
		}
		attachment.ID = id
	}
	if len(attachment.CryptoMetadata) == 0 {
		attachment.CryptoMetadata = json.RawMessage(`{}`)
	}
	attachment.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO attachment_envelopes(id, owner_account_id, conversation_id, storage_key, ciphertext_sha256, size_bytes, crypto_metadata_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, attachment.ID, attachment.OwnerAccountID, nullableString(attachment.ConversationID), attachment.StorageKey, attachment.CiphertextSHA256, attachment.SizeBytes, string(attachment.CryptoMetadata), formatTime(attachment.CreatedAt))
	return attachment, err
}

func (s *Store) CreateReaction(ctx context.Context, messageID, accountID string, reactionCiphertext []byte) error {
	msg, err := s.MessageByID(ctx, messageID)
	if err != nil {
		return err
	}
	member, err := s.IsConversationMember(ctx, msg.ConversationID, accountID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotMember
	}
	id, err := domain.NewID("react")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO reactions(id, message_id, account_id, reaction_ciphertext, created_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(message_id, account_id) DO UPDATE SET reaction_ciphertext = excluded.reaction_ciphertext, created_at = excluded.created_at`, id, messageID, accountID, reactionCiphertext, nowString())
	return err
}

func (s *Store) MarkRead(ctx context.Context, conversationID, accountID, messageID string) error {
	member, err := s.IsConversationMember(ctx, conversationID, accountID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotMember
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_envelopes WHERE id = ? AND conversation_id = ?`, messageID, conversationID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO read_receipts(account_id, conversation_id, message_id, read_at) VALUES(?, ?, ?, ?) ON CONFLICT(account_id, conversation_id) DO UPDATE SET message_id = excluded.message_id, read_at = excluded.read_at`, accountID, conversationID, messageID, nowString())
	return err
}

func (s *Store) CreatePushSubscription(ctx context.Context, accountID, deviceID, provider, endpoint string) error {
	id, err := domain.NewID("push")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO push_subscriptions(id, account_id, device_id, provider, endpoint, created_at) VALUES(?, ?, ?, ?, ?, ?)`, id, accountID, nullableEmptyString(deviceID), provider, endpoint, nowString())
	return err
}

func (s *Store) CreateCallSession(ctx context.Context, conversationID, accountID string, metadata json.RawMessage) (domain.CallSession, error) {
	member, err := s.IsConversationMember(ctx, conversationID, accountID)
	if err != nil {
		return domain.CallSession{}, err
	}
	if !member {
		return domain.CallSession{}, ErrNotMember
	}
	id, err := domain.NewID("call")
	if err != nil {
		return domain.CallSession{}, err
	}
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	createdAt := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO call_sessions(id, conversation_id, created_by, state, metadata_json, created_at) VALUES(?, ?, ?, 'ringing', ?, ?)`, id, conversationID, accountID, string(metadata), formatTime(createdAt))
	if err != nil {
		return domain.CallSession{}, err
	}
	return domain.CallSession{ID: id, ConversationID: conversationID, CreatedBy: accountID, State: "ringing", Metadata: metadata, CreatedAt: createdAt}, nil
}

func (s *Store) CreateBackupBlob(ctx context.Context, accountID, deviceID, storageKey string, sizeBytes int64, keyDerivationMetadata json.RawMessage) error {
	if len(keyDerivationMetadata) == 0 {
		keyDerivationMetadata = json.RawMessage(`{}`)
	}
	id, err := domain.NewID("backup")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO backup_blobs(id, account_id, device_id, storage_key, size_bytes, key_derivation_metadata_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, id, accountID, nullableEmptyString(deviceID), storageKey, sizeBytes, string(keyDerivationMetadata), nowString())
	return err
}

func (s *Store) ExportAccount(ctx context.Context, accountID string) (domain.AccountExport, error) {
	account, err := s.accountByID(ctx, accountID)
	if err != nil {
		return domain.AccountExport{}, err
	}
	devices, err := s.ListDevices(ctx, accountID)
	if err != nil {
		return domain.AccountExport{}, err
	}
	conversations, err := s.ListConversations(ctx, accountID)
	if err != nil {
		return domain.AccountExport{}, err
	}
	messages, err := s.listVisibleMessagesForExport(ctx, accountID)
	if err != nil {
		return domain.AccountExport{}, err
	}
	return domain.AccountExport{Account: account, Devices: devices, Conversations: conversations, Messages: messages}, nil
}

func (s *Store) DeleteAccount(ctx context.Context, accountID string) error {
	now := nowString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE account_id = ?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET revoked_at = COALESCE(revoked_at, ?) WHERE account_id = ?`, now, accountID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET status = 'deleted', deleted_at = COALESCE(deleted_at, ?) WHERE id = ?`, now, accountID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) accountByID(ctx context.Context, accountID string) (domain.Account, error) {
	var account domain.Account
	var email, deleted sql.NullString
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, email, role, status, created_at, deleted_at FROM accounts WHERE id = ?`, accountID).Scan(&account.ID, &account.Username, &email, &account.Role, &account.Status, &created, &deleted)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Account{}, ErrNotFound
		}
		return domain.Account{}, err
	}
	account.Email = stringPtr(email)
	account.CreatedAt = parseTime(created)
	account.DeletedAt = parseOptionalTime(deleted)
	return account, nil
}

func (s *Store) listVisibleMessagesForExport(ctx context.Context, accountID string) ([]domain.MessageEnvelope, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT me.id, me.conversation_id, me.sender_account_id, me.sender_device_id, me.idempotency_key, me.ciphertext, me.crypto_protocol, me.crypto_metadata_json, me.attachment_refs_json, me.reply_to_id, me.thread_root_id, me.created_at, me.edited_at, me.deleted_at, me.expires_at
		FROM message_envelopes me
		JOIN memberships m ON m.conversation_id = me.conversation_id
		WHERE m.account_id = ?
		ORDER BY me.created_at DESC
		LIMIT 1000`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []domain.MessageEnvelope
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanDevice(rows scanner) (domain.Device, error) {
	var device domain.Device
	var signing []byte
	var lastSeen, revoked sql.NullString
	var created string
	if err := rows.Scan(&device.ID, &device.AccountID, &device.Name, &device.KeyPackage, &signing, &created, &lastSeen, &revoked); err != nil {
		return domain.Device{}, err
	}
	device.SigningKey = signing
	device.CreatedAt = parseTime(created)
	device.LastSeenAt = parseOptionalTime(lastSeen)
	device.RevokedAt = parseOptionalTime(revoked)
	return device, nil
}

func scanConversation(rows scanner) (domain.Conversation, error) {
	var c domain.Conversation
	var title, communityID, channelID, created sql.NullString
	var retention sql.NullInt64
	if err := rows.Scan(&c.ID, &c.Kind, &title, &communityID, &channelID, &c.CreatedBy, &retention, &created); err != nil {
		return domain.Conversation{}, err
	}
	c.Title = stringPtr(title)
	c.CommunityID = stringPtr(communityID)
	c.ChannelID = stringPtr(channelID)
	if retention.Valid {
		c.RetentionSeconds = &retention.Int64
	}
	c.CreatedAt = parseTime(created.String)
	return c, nil
}

func scanMessage(rows scanner) (domain.MessageEnvelope, error) {
	var msg domain.MessageEnvelope
	var cryptoMetadata, attachmentRefs string
	var replyTo, threadRoot, created, edited, deleted, expires sql.NullString
	if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.SenderAccountID, &msg.SenderDeviceID, &msg.IdempotencyKey, &msg.Ciphertext, &msg.CryptoProtocol, &cryptoMetadata, &attachmentRefs, &replyTo, &threadRoot, &created, &edited, &deleted, &expires); err != nil {
		return domain.MessageEnvelope{}, err
	}
	msg.CryptoMetadata = json.RawMessage(cryptoMetadata)
	msg.AttachmentRefs = json.RawMessage(attachmentRefs)
	msg.ReplyToID = stringPtr(replyTo)
	msg.ThreadRootID = stringPtr(threadRoot)
	msg.CreatedAt = parseTime(created.String)
	msg.EditedAt = parseOptionalTime(edited)
	msg.DeletedAt = parseOptionalTime(deleted)
	msg.ExpiresAt = parseOptionalTime(expires)
	return msg, nil
}

func scanSyncEvent(rows scanner) (domain.SyncEvent, error) {
	var event domain.SyncEvent
	var accountID, conversationID, created sql.NullString
	var payload string
	if err := rows.Scan(&event.ID, &event.Type, &accountID, &conversationID, &payload, &created); err != nil {
		return domain.SyncEvent{}, err
	}
	event.AccountID = stringPtr(accountID)
	if conversationID.Valid {
		event.ConversationID = conversationID.String
	}
	event.Payload = json.RawMessage(payload)
	event.CreatedAt = parseTime(created.String)
	return event, nil
}

func nullableString(value *string) sql.NullString {
	if value == nil || strings.TrimSpace(*value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(*value), Valid: true}
}

func nullableEmptyString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(value), Valid: true}
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(value.UTC()), Valid: true}
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func parseOptionalTime(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	t := parseTime(value.String)
	return &t
}

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func nowString() string {
	return formatTime(time.Now().UTC())
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}
