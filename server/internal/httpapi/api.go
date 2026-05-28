package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"private-messenger/server/internal/auth"
	"private-messenger/server/internal/domain"
	"private-messenger/server/internal/realtime"
	"private-messenger/server/internal/storage"
	"private-messenger/server/internal/uploads"
	"private-messenger/server/websetup"
)

type API struct {
	Store *storage.Store
	Hub   *realtime.Hub
	Blobs uploads.Store
	Log   *slog.Logger
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/health", a.health)
	mux.HandleFunc("GET /setup", a.setupPage)
	mux.HandleFunc("GET /api/v1/setup/status", a.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/owner", a.createOwner)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/register", a.register)
	mux.HandleFunc("POST /api/v1/invites", a.withAuth(a.createInvite))
	mux.HandleFunc("GET /api/v1/devices/me", a.withAuth(a.listDevices))
	mux.HandleFunc("POST /api/v1/communities", a.withAuth(a.createCommunity))
	mux.HandleFunc("POST /api/v1/conversations", a.withAuth(a.createConversation))
	mux.HandleFunc("GET /api/v1/conversations", a.withAuth(a.listConversations))
	mux.HandleFunc("POST /api/v1/messages/envelopes", a.withAuth(a.createMessageEnvelope))
	mux.HandleFunc("POST /api/v1/attachments", a.withAuth(a.uploadAttachment))
	mux.HandleFunc("POST /api/v1/push/subscriptions", a.withAuth(a.createPushSubscription))
	mux.HandleFunc("POST /api/v1/calls", a.withAuth(a.createCall))
	mux.HandleFunc("GET /api/v1/sync/ws", a.syncWebSocket)
	mux.HandleFunc("GET /api/v1/sync/events", a.withAuth(a.syncEvents))
	mux.HandleFunc("GET /api/v1/search/metadata", a.withAuth(a.searchMetadata))
	mux.HandleFunc("GET /api/v1/account/export", a.withAuth(a.exportAccount))
	mux.HandleFunc("DELETE /api/v1/account", a.withAuth(a.deleteAccount))
	mux.HandleFunc("POST /api/v1/backups", a.withAuth(a.uploadBackup))
	mux.HandleFunc("/api/v1/messages/", a.withAuth(a.messageSubroute))
	mux.HandleFunc("/api/v1/conversations/", a.withAuth(a.conversationSubroute))
	mux.HandleFunc("/api/v1/communities/", a.withAuth(a.communitySubroute))
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.Check(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) setupPage(w http.ResponseWriter, r *http.Request) {
	page, err := websetup.FS.ReadFile("index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "setup_ui_unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (a *API) setupStatus(w http.ResponseWriter, r *http.Request) {
	required, err := a.Store.SetupRequired(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "setup_status_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"setup_required": required})
}

type ownerRequest struct {
	InstanceName     string  `json:"instance_name"`
	Username         string  `json:"username"`
	Email            *string `json:"email,omitempty"`
	Password         string  `json:"password"`
	DeviceName       string  `json:"device_name"`
	DeviceKeyPackage []byte  `json:"device_key_package"`
}

func (a *API) createOwner(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Private-Messenger-Setup") != "1" {
		writeError(w, http.StatusForbidden, "setup_csrf_guard_required")
		return
	}
	var req ownerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "weak_password")
		return
	}
	if len(req.DeviceKeyPackage) == 0 {
		writeError(w, http.StatusBadRequest, "device_key_package_required")
		return
	}
	created, err := a.Store.CreateOwner(r.Context(), storage.CreateOwnerInput{
		InstanceName: req.InstanceName,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		DeviceName:   req.DeviceName,
		KeyPackage:   req.DeviceKeyPackage,
	})
	if err != nil {
		if errors.Is(err, storage.ErrAlreadySetup) {
			writeError(w, http.StatusConflict, "already_setup")
			return
		}
		writeError(w, http.StatusInternalServerError, "owner_create_failed")
		return
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_create_failed")
		return
	}
	if err := a.Store.CreateSession(r.Context(), tokenHash, created.Account.ID, created.Device.ID, time.Now().UTC().Add(30*24*time.Hour)); err != nil {
		writeError(w, http.StatusInternalServerError, "session_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"account": created.Account, "device": created.Device, "token": token})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"device_id,omitempty"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	record, err := a.Store.LoginRecord(r.Context(), req.Username, req.DeviceID)
	if err != nil || !auth.VerifyPassword(record.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_create_failed")
		return
	}
	if err := a.Store.CreateSession(r.Context(), tokenHash, record.AccountID, record.DeviceID, time.Now().UTC().Add(30*24*time.Hour)); err != nil {
		writeError(w, http.StatusInternalServerError, "session_create_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "account_id": record.AccountID, "device_id": record.DeviceID, "role": record.Role})
}

type registerRequest struct {
	InviteCode       string  `json:"invite_code"`
	Username         string  `json:"username"`
	Email            *string `json:"email,omitempty"`
	Password         string  `json:"password"`
	DeviceName       string  `json:"device_name"`
	DeviceKeyPackage []byte  `json:"device_key_package"`
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "weak_password")
		return
	}
	if len(req.DeviceKeyPackage) == 0 {
		writeError(w, http.StatusBadRequest, "device_key_package_required")
		return
	}
	created, err := a.Store.RegisterWithInvite(r.Context(), storage.RegisterInput{
		InviteCode:   req.InviteCode,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		DeviceName:   req.DeviceName,
		KeyPackage:   req.DeviceKeyPackage,
	})
	if err != nil {
		if errors.Is(err, storage.ErrInviteInvalid) {
			writeError(w, http.StatusBadRequest, "invalid_invite")
			return
		}
		writeError(w, http.StatusInternalServerError, "register_failed")
		return
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_create_failed")
		return
	}
	if err := a.Store.CreateSession(r.Context(), tokenHash, created.Account.ID, created.Device.ID, time.Now().UTC().Add(30*24*time.Hour)); err != nil {
		writeError(w, http.StatusInternalServerError, "session_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"account": created.Account, "device": created.Device, "token": token})
}

type inviteRequest struct {
	MaxUses   int        `json:"max_uses"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (a *API) createInvite(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	if !domain.CanManageInvites(principal.Role) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req inviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	invite, err := a.Store.CreateInvite(r.Context(), principal.AccountID, req.MaxUses, req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invite_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (a *API) listDevices(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	devices, err := a.Store.ListDevices(r.Context(), principal.AccountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "devices_list_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

type createCommunityRequest struct {
	Name string `json:"name"`
}

func (a *API) createCommunity(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	var req createCommunityRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	community, err := a.Store.CreateCommunity(r.Context(), req.Name, principal.AccountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "community_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, community)
}

func (a *API) communitySubroute(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/communities/"), "/")
	if len(parts) == 2 && parts[1] == "channels" && r.Method == http.MethodPost {
		var req struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		channel, err := a.Store.CreateChannel(r.Context(), parts[0], req.Name, req.Kind, principal.AccountID)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, channel)
		return
	}
	writeError(w, http.StatusNotFound, "not_found")
}

type createConversationRequest struct {
	Kind             string   `json:"kind"`
	Title            *string  `json:"title,omitempty"`
	CommunityID      *string  `json:"community_id,omitempty"`
	ChannelID        *string  `json:"channel_id,omitempty"`
	RetentionSeconds *int64   `json:"retention_seconds,omitempty"`
	MemberAccountIDs []string `json:"member_account_ids,omitempty"`
}

func (a *API) createConversation(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	var req createConversationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Kind != "dm" && req.Kind != "group" && req.Kind != "community_channel" {
		writeError(w, http.StatusBadRequest, "invalid_conversation_kind")
		return
	}
	conversation, err := a.Store.CreateConversation(r.Context(), storage.CreateConversationInput{
		Kind:             req.Kind,
		Title:            req.Title,
		CommunityID:      req.CommunityID,
		ChannelID:        req.ChannelID,
		CreatedBy:        principal.AccountID,
		RetentionSeconds: req.RetentionSeconds,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "conversation_create_failed")
		return
	}
	for _, accountID := range req.MemberAccountIDs {
		if accountID != "" && accountID != principal.AccountID {
			if err := a.Store.AddConversationMember(r.Context(), conversation.ID, accountID, domain.RoleMember); err != nil {
				handleStorageError(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusCreated, conversation)
}

func (a *API) listConversations(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	conversations, err := a.Store.ListConversations(r.Context(), principal.AccountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "conversations_list_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"conversations": conversations})
}

func (a *API) conversationSubroute(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/conversations/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	conversationID := parts[0]
	switch {
	case parts[1] == "members" && r.Method == http.MethodPost:
		canManage, err := a.canManageConversation(r.Context(), conversationID, principal)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		if !canManage {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		var req struct {
			AccountID string `json:"account_id"`
			Role      string `json:"role"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := a.Store.AddConversationMember(r.Context(), conversationID, req.AccountID, req.Role); err != nil {
			writeError(w, http.StatusInternalServerError, "member_add_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case parts[1] == "messages" && r.Method == http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		messages, err := a.Store.ListMessages(r.Context(), conversationID, principal.AccountID, limit)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"messages": messages})
	case parts[1] == "typing" && r.Method == http.MethodPost:
		members, err := a.Store.ListConversationMemberIDs(r.Context(), conversationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "typing_publish_failed")
			return
		}
		a.Hub.Publish(members, realtime.Event{Version: "v1", Type: "typing.updated", ConversationID: conversationID, Payload: map[string]string{"account_id": principal.AccountID}, CreatedAt: time.Now().UTC()})
		w.WriteHeader(http.StatusNoContent)
	case parts[1] == "read-receipts" && r.Method == http.MethodPost:
		var req struct {
			MessageID string `json:"message_id"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.MessageID == "" {
			writeError(w, http.StatusBadRequest, "message_id_required")
			return
		}
		if err := a.Store.MarkRead(r.Context(), conversationID, principal.AccountID, req.MessageID); err != nil {
			handleStorageError(w, err)
			return
		}
		payload := map[string]string{"account_id": principal.AccountID, "message_id": req.MessageID}
		eventID, _ := a.Store.SaveSyncEvent(r.Context(), "read_receipt.updated", nil, conversationID, payload)
		members, _ := a.Store.ListConversationMemberIDs(r.Context(), conversationID)
		a.Hub.Publish(members, realtime.Event{Version: "v1", Type: "read_receipt.updated", ID: eventID, ConversationID: conversationID, Payload: payload, CreatedAt: time.Now().UTC()})
		w.WriteHeader(http.StatusNoContent)
	case parts[1] == "retention" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		var req struct {
			RetentionSeconds *int64 `json:"retention_seconds"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		conversation, err := a.Store.UpdateConversationRetention(r.Context(), conversationID, principal.AccountID, req.RetentionSeconds)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		eventID, _ := a.Store.SaveSyncEvent(r.Context(), "retention.updated", nil, conversationID, conversation)
		members, _ := a.Store.ListConversationMemberIDs(r.Context(), conversationID)
		a.Hub.Publish(members, realtime.Event{Version: "v1", Type: "retention.updated", ID: eventID, ConversationID: conversationID, Payload: conversation, CreatedAt: time.Now().UTC()})
		writeJSON(w, http.StatusOK, conversation)
	default:
		writeError(w, http.StatusNotFound, "not_found")
	}
}

type messageEnvelopeRequest struct {
	ConversationID string          `json:"conversation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Ciphertext     []byte          `json:"ciphertext"`
	CryptoProtocol string          `json:"crypto_protocol"`
	CryptoMetadata json.RawMessage `json:"crypto_metadata,omitempty"`
	AttachmentRefs json.RawMessage `json:"attachment_refs,omitempty"`
	ReplyToID      *string         `json:"reply_to_id,omitempty"`
	ThreadRootID   *string         `json:"thread_root_id,omitempty"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
}

func (a *API) createMessageEnvelope(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	raw, ok := readLimitedJSON(w, r)
	if !ok {
		return
	}
	if containsPlaintextMessageKey(raw) {
		writeError(w, http.StatusBadRequest, "plaintext_message_fields_forbidden")
		return
	}
	var req messageEnvelopeRequest
	if !decodeRawJSON(w, raw, &req) {
		return
	}
	if principal.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "device_session_required")
		return
	}
	if req.ConversationID == "" || req.IdempotencyKey == "" || len(req.Ciphertext) == 0 || req.CryptoProtocol == "" {
		writeError(w, http.StatusBadRequest, "invalid_encrypted_envelope")
		return
	}
	envelope, duplicate, err := a.Store.SaveMessageEnvelope(r.Context(), domain.MessageEnvelope{
		ConversationID:  req.ConversationID,
		SenderAccountID: principal.AccountID,
		SenderDeviceID:  principal.DeviceID,
		IdempotencyKey:  req.IdempotencyKey,
		Ciphertext:      req.Ciphertext,
		CryptoProtocol:  req.CryptoProtocol,
		CryptoMetadata:  req.CryptoMetadata,
		AttachmentRefs:  req.AttachmentRefs,
		ReplyToID:       req.ReplyToID,
		ThreadRootID:    req.ThreadRootID,
		ExpiresAt:       req.ExpiresAt,
	})
	if err != nil {
		handleStorageError(w, err)
		return
	}
	eventID, _ := a.Store.SaveSyncEvent(r.Context(), "message.envelope.created", nil, envelope.ConversationID, envelope)
	members, _ := a.Store.ListConversationMemberIDs(r.Context(), envelope.ConversationID)
	a.Hub.Publish(members, realtime.Event{Version: "v1", Type: "message.envelope.created", ID: eventID, ConversationID: envelope.ConversationID, Payload: envelope, CreatedAt: time.Now().UTC()})
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, envelope)
}

type encryptedMessageMutationRequest struct {
	Ciphertext     []byte          `json:"ciphertext"`
	CryptoProtocol string          `json:"crypto_protocol"`
	CryptoMetadata json.RawMessage `json:"crypto_metadata,omitempty"`
}

func (a *API) messageSubroute(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/messages/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	messageID := parts[0]
	switch {
	case parts[1] == "edit" && r.Method == http.MethodPost:
		req, ok := decodeEncryptedMutation(w, r)
		if !ok {
			return
		}
		envelope, err := a.Store.UpdateMessageEnvelope(r.Context(), messageID, principal.AccountID, req.Ciphertext, req.CryptoProtocol, req.CryptoMetadata)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		a.publishMessageEvent(r, "message.envelope.edited", envelope)
		writeJSON(w, http.StatusOK, envelope)
	case parts[1] == "delete" && r.Method == http.MethodPost:
		req, ok := decodeEncryptedMutation(w, r)
		if !ok {
			return
		}
		envelope, err := a.Store.DeleteMessageEnvelope(r.Context(), messageID, principal.AccountID, req.Ciphertext, req.CryptoProtocol, req.CryptoMetadata)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		a.publishMessageEvent(r, "message.envelope.deleted", envelope)
		writeJSON(w, http.StatusOK, envelope)
	case parts[1] == "reactions" && r.Method == http.MethodPost:
		raw, ok := readLimitedJSON(w, r)
		if !ok {
			return
		}
		if containsPlaintextMessageKey(raw) {
			writeError(w, http.StatusBadRequest, "plaintext_message_fields_forbidden")
			return
		}
		var req struct {
			ReactionCiphertext []byte `json:"reaction_ciphertext"`
		}
		if !decodeRawJSON(w, raw, &req) {
			return
		}
		if len(req.ReactionCiphertext) == 0 {
			writeError(w, http.StatusBadRequest, "reaction_ciphertext_required")
			return
		}
		if err := a.Store.CreateReaction(r.Context(), messageID, principal.AccountID, req.ReactionCiphertext); err != nil {
			handleStorageError(w, err)
			return
		}
		message, err := a.Store.MessageByID(r.Context(), messageID)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		payload := map[string]string{"message_id": messageID, "account_id": principal.AccountID}
		eventID, _ := a.Store.SaveSyncEvent(r.Context(), "reaction.created", nil, message.ConversationID, payload)
		members, _ := a.Store.ListConversationMemberIDs(r.Context(), message.ConversationID)
		a.Hub.Publish(members, realtime.Event{Version: "v1", Type: "reaction.created", ID: eventID, ConversationID: message.ConversationID, Payload: payload, CreatedAt: time.Now().UTC()})
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusNotFound, "not_found")
	}
}

func decodeEncryptedMutation(w http.ResponseWriter, r *http.Request) (encryptedMessageMutationRequest, bool) {
	raw, ok := readLimitedJSON(w, r)
	if !ok {
		return encryptedMessageMutationRequest{}, false
	}
	if containsPlaintextMessageKey(raw) {
		writeError(w, http.StatusBadRequest, "plaintext_message_fields_forbidden")
		return encryptedMessageMutationRequest{}, false
	}
	var req encryptedMessageMutationRequest
	if !decodeRawJSON(w, raw, &req) {
		return encryptedMessageMutationRequest{}, false
	}
	if len(req.Ciphertext) == 0 || req.CryptoProtocol == "" {
		writeError(w, http.StatusBadRequest, "invalid_encrypted_marker")
		return encryptedMessageMutationRequest{}, false
	}
	return req, true
}

func (a *API) publishMessageEvent(r *http.Request, eventType string, envelope domain.MessageEnvelope) {
	eventID, _ := a.Store.SaveSyncEvent(r.Context(), eventType, nil, envelope.ConversationID, envelope)
	members, _ := a.Store.ListConversationMemberIDs(r.Context(), envelope.ConversationID)
	a.Hub.Publish(members, realtime.Event{Version: "v1", Type: eventType, ID: eventID, ConversationID: envelope.ConversationID, Payload: envelope, CreatedAt: time.Now().UTC()})
}

func (a *API) uploadAttachment(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	if r.Header.Get("X-Private-Messenger-Encrypted") != "1" {
		writeError(w, http.StatusBadRequest, "encrypted_upload_header_required")
		return
	}
	conversationID := optionalQuery(r, "conversation_id")
	if conversationID != nil {
		member, err := a.Store.IsConversationMember(r.Context(), *conversationID, principal.AccountID)
		if err != nil {
			handleStorageError(w, err)
			return
		}
		if !member {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	storageKey, sha, size, err := a.Blobs.PutEncryptedBlob(r.Context(), http.MaxBytesReader(w, r.Body, 50<<20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_failed")
		return
	}
	metadata := json.RawMessage(r.Header.Get("X-Crypto-Metadata"))
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		writeError(w, http.StatusBadRequest, "invalid_crypto_metadata")
		return
	}
	attachment, err := a.Store.CreateAttachmentEnvelope(r.Context(), domain.AttachmentEnvelope{
		OwnerAccountID:   principal.AccountID,
		ConversationID:   conversationID,
		StorageKey:       storageKey,
		CiphertextSHA256: sha,
		SizeBytes:        size,
		CryptoMetadata:   metadata,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "attachment_record_failed")
		return
	}
	writeJSON(w, http.StatusCreated, attachment)
}

func (a *API) uploadBackup(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	if r.Header.Get("X-Private-Messenger-Encrypted") != "1" {
		writeError(w, http.StatusBadRequest, "encrypted_upload_header_required")
		return
	}
	metadata := json.RawMessage(r.Header.Get("X-Key-Derivation-Metadata"))
	if len(metadata) == 0 {
		writeError(w, http.StatusBadRequest, "key_derivation_metadata_required")
		return
	}
	if !json.Valid(metadata) {
		writeError(w, http.StatusBadRequest, "invalid_key_derivation_metadata")
		return
	}
	storageKey, sha, size, err := a.Blobs.PutEncryptedBlob(r.Context(), http.MaxBytesReader(w, r.Body, 100<<20))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup_upload_failed")
		return
	}
	if err := a.Store.CreateBackupBlob(r.Context(), principal.AccountID, principal.DeviceID, storageKey, size, metadata); err != nil {
		writeError(w, http.StatusInternalServerError, "backup_record_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"storage_key": storageKey, "ciphertext_sha256": sha, "size_bytes": size})
}

func (a *API) createPushSubscription(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	var req struct {
		Provider string `json:"provider"`
		Endpoint string `json:"endpoint"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Provider == "" || req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "invalid_push_subscription")
		return
	}
	if err := a.Store.CreatePushSubscription(r.Context(), principal.AccountID, principal.DeviceID, req.Provider, req.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "push_subscription_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok", "payload_policy": "generic_encrypted_event_only"})
}

func (a *API) createCall(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	var req struct {
		ConversationID string          `json:"conversation_id"`
		Metadata       json.RawMessage `json:"metadata,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	call, err := a.Store.CreateCallSession(r.Context(), req.ConversationID, principal.AccountID, req.Metadata)
	if err != nil {
		handleStorageError(w, err)
		return
	}
	eventID, _ := a.Store.SaveSyncEvent(r.Context(), "call.signaling", nil, req.ConversationID, call)
	members, _ := a.Store.ListConversationMemberIDs(r.Context(), req.ConversationID)
	a.Hub.Publish(members, realtime.Event{Version: "v1", Type: "call.signaling", ID: eventID, ConversationID: req.ConversationID, Payload: call, CreatedAt: time.Now().UTC()})
	writeJSON(w, http.StatusCreated, call)
}

func (a *API) syncEvents(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := a.Store.ListSyncEvents(r.Context(), principal.AccountID, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync_events_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

func (a *API) searchMetadata(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	results, err := a.Store.SearchMetadata(r.Context(), principal.AccountID, r.URL.Query().Get("q"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}

func (a *API) exportAccount(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	export, err := a.Store.ExportAccount(r.Context(), principal.AccountID)
	if err != nil {
		handleStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, export)
}

func (a *API) deleteAccount(w http.ResponseWriter, r *http.Request, principal domain.Principal) {
	if err := a.Store.DeleteAccount(r.Context(), principal.AccountID); err != nil {
		handleStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) syncWebSocket(w http.ResponseWriter, r *http.Request) {
	principal, err := a.principalFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	client := a.Hub.Register(principal.AccountID)
	_ = realtime.ServeWebSocket(w, r, client, func() { a.Hub.Unregister(client) })
}

type authedHandler func(http.ResponseWriter, *http.Request, domain.Principal)

func (a *API) withAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.principalFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r, principal)
	}
}

func (a *API) principalFromRequest(r *http.Request) (domain.Principal, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return domain.Principal{}, storage.ErrUnauthorized
	}
	return a.Store.PrincipalByTokenHash(r.Context(), auth.HashToken(token))
}

func (a *API) canManageConversation(ctx context.Context, conversationID string, principal domain.Principal) (bool, error) {
	if domain.CanManageMembers(principal.Role) {
		return true, nil
	}
	role, err := a.Store.ConversationMemberRole(ctx, conversationID, principal.AccountID)
	if err != nil {
		return false, err
	}
	return domain.CanManageMembers(role), nil
}

func readLimitedJSON(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body")
		return nil, false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		writeError(w, http.StatusBadRequest, "empty_body")
		return nil, false
	}
	return raw, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	raw, ok := readLimitedJSON(w, r)
	if !ok {
		return false
	}
	return decodeRawJSON(w, raw, dest)
}

func decodeRawJSON(w http.ResponseWriter, raw []byte, dest interface{}) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func containsPlaintextMessageKey(raw []byte) bool {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return containsForbiddenPlaintextKey(value)
}

func containsForbiddenPlaintextKey(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			if isForbiddenPlaintextKey(key) || containsForbiddenPlaintextKey(nested) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if containsForbiddenPlaintextKey(nested) {
				return true
			}
		}
	}
	return false
}

func isForbiddenPlaintextKey(key string) bool {
	switch strings.ToLower(key) {
	case "plaintext", "plain_text", "body", "text", "message", "message_text", "content":
		return true
	default:
		return false
	}
}

func optionalQuery(r *http.Request, name string) *string {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return nil
	}
	return &value
}

func handleStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotMember), errors.Is(err, storage.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	default:
		writeError(w, http.StatusInternalServerError, "storage_error")
	}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
