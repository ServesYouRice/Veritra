package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"private-messenger/server/internal/app"
	"private-messenger/server/internal/config"
)

func TestMessageAPIRejectsPlaintextFields(t *testing.T) {
	handler, token, dbPath := newTestHandlerWithOwner(t)
	conversationID := createConversation(t, handler, token)
	sentinel := runtimeSentinel(t)

	body := map[string]interface{}{
		"conversation_id": conversationID,
		"idempotency_key": "send-plaintext",
		"body":            sentinel,
		"ciphertext":      base64.StdEncoding.EncodeToString([]byte("ciphertext")),
		"crypto_protocol": "mls-openmls-todo",
	}
	status, _ := doJSON(t, handler, http.MethodPost, "/api/v1/messages/envelopes", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", status, http.StatusBadRequest)
	}
	nested := map[string]interface{}{
		"conversation_id": conversationID,
		"idempotency_key": "send-nested-plaintext",
		"ciphertext":      base64.StdEncoding.EncodeToString([]byte("ciphertext")),
		"crypto_protocol": "mls-openmls-todo",
		"crypto_metadata": map[string]interface{}{"body": sentinel},
	}
	status, _ = doJSON(t, handler, http.MethodPost, "/api/v1/messages/envelopes", token, nested)
	if status != http.StatusBadRequest {
		t.Fatalf("nested status=%d want %d", status, http.StatusBadRequest)
	}
	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if bytes.Contains(dbBytes, []byte(sentinel)) {
		t.Fatal("rejected plaintext sentinel was persisted")
	}
}

func runtimeSentinel(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return "PLAINTEXT_SENTINEL_" + hex.EncodeToString(b[:])
}

func TestMessageAPIAcceptsCiphertextEnvelope(t *testing.T) {
	handler, token, _ := newTestHandlerWithOwner(t)
	conversationID := createConversation(t, handler, token)
	body := map[string]interface{}{
		"conversation_id": conversationID,
		"idempotency_key": "send-ciphertext",
		"ciphertext":      base64.StdEncoding.EncodeToString([]byte("ciphertext")),
		"crypto_protocol": "mls-openmls-todo",
	}
	status, response := doJSON(t, handler, http.MethodPost, "/api/v1/messages/envelopes", token, body)
	if status != http.StatusCreated {
		t.Fatalf("status=%d body=%s", status, response)
	}
}

func TestMessageLifecycleAndSyncRoutes(t *testing.T) {
	handler, token, _ := newTestHandlerWithOwner(t)
	conversationID := createConversation(t, handler, token)
	messageID := createMessage(t, handler, token, conversationID, "lifecycle-1", []byte("ciphertext-v1"))

	status, response := doJSON(t, handler, http.MethodPost, "/api/v1/messages/"+messageID+"/edit", token, map[string]interface{}{
		"ciphertext":      base64.StdEncoding.EncodeToString([]byte("ciphertext-v2")),
		"crypto_protocol": "mls-openmls-todo",
	})
	if status != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", status, response)
	}
	if !bytes.Contains(response, []byte(`"edited_at"`)) {
		t.Fatalf("edit response missing edited marker: %s", response)
	}

	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/messages/"+messageID+"/reactions", token, map[string]interface{}{
		"reaction_ciphertext": base64.StdEncoding.EncodeToString([]byte("encrypted reaction")),
	})
	if status != http.StatusNoContent {
		t.Fatalf("reaction status=%d body=%s", status, response)
	}

	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/read-receipts", token, map[string]interface{}{"message_id": messageID})
	if status != http.StatusNoContent {
		t.Fatalf("read receipt status=%d body=%s", status, response)
	}

	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/messages/"+messageID+"/delete", token, map[string]interface{}{
		"ciphertext":      base64.StdEncoding.EncodeToString([]byte("encrypted delete marker")),
		"crypto_protocol": "mls-openmls-todo",
	})
	if status != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", status, response)
	}
	if !bytes.Contains(response, []byte(`"deleted_at"`)) {
		t.Fatalf("delete response missing deleted marker: %s", response)
	}

	status, response = doJSON(t, handler, http.MethodGet, "/api/v1/sync/events?after=0", token, nil)
	if status != http.StatusOK {
		t.Fatalf("sync events status=%d body=%s", status, response)
	}
	for _, eventType := range []string{"message.envelope.created", "message.envelope.edited", "reaction.created", "read_receipt.updated", "message.envelope.deleted"} {
		if !bytes.Contains(response, []byte(eventType)) {
			t.Fatalf("sync response missing %s: %s", eventType, response)
		}
	}
}

func TestConversationScopedWritesRequireMembership(t *testing.T) {
	handler, ownerToken, _ := newTestHandlerWithOwner(t)
	conversationID := createConversation(t, handler, ownerToken)
	messageID := createMessage(t, handler, ownerToken, conversationID, "owner-only", []byte("ciphertext"))
	memberToken := registerMember(t, handler, ownerToken, "outsider")

	status, response := doRaw(t, handler, http.MethodPost, "/api/v1/attachments?conversation_id="+conversationID, memberToken, []byte("encrypted attachment"), map[string]string{
		"X-Private-Messenger-Encrypted": "1",
		"X-Crypto-Metadata":             "{}",
	})
	if status != http.StatusForbidden {
		t.Fatalf("attachment status=%d body=%s", status, response)
	}

	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/calls", memberToken, map[string]interface{}{"conversation_id": conversationID})
	if status != http.StatusForbidden {
		t.Fatalf("call status=%d body=%s", status, response)
	}

	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/messages/"+messageID+"/reactions", memberToken, map[string]interface{}{
		"reaction_ciphertext": base64.StdEncoding.EncodeToString([]byte("encrypted reaction")),
	})
	if status != http.StatusForbidden {
		t.Fatalf("reaction status=%d body=%s", status, response)
	}

	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/read-receipts", memberToken, map[string]interface{}{"message_id": messageID})
	if status != http.StatusForbidden {
		t.Fatalf("read receipt status=%d body=%s", status, response)
	}
}

func TestMetadataSearchBackupExportAndAccountDelete(t *testing.T) {
	handler, token, _ := newTestHandlerWithOwner(t)
	conversationID := createConversation(t, handler, token)
	searchOnlyCiphertext := []byte("SEARCH_ONLY_CIPHERTEXT")
	createMessage(t, handler, token, conversationID, "search-only", searchOnlyCiphertext)

	status, response := doJSON(t, handler, http.MethodGet, "/api/v1/search/metadata?q=owner", token, nil)
	if status != http.StatusOK {
		t.Fatalf("search status=%d body=%s", status, response)
	}
	if !bytes.Contains(response, []byte(`"type":"account"`)) {
		t.Fatalf("search did not include account metadata: %s", response)
	}
	status, response = doJSON(t, handler, http.MethodGet, "/api/v1/search/metadata?q=SEARCH_ONLY_CIPHERTEXT", token, nil)
	if status != http.StatusOK {
		t.Fatalf("ciphertext search status=%d body=%s", status, response)
	}
	if bytes.Contains(response, []byte("SEARCH_ONLY_CIPHERTEXT")) {
		t.Fatalf("metadata search returned ciphertext content: %s", response)
	}

	status, response = doRaw(t, handler, http.MethodPost, "/api/v1/backups", token, []byte("encrypted backup blob"), map[string]string{
		"X-Private-Messenger-Encrypted": "1",
		"X-Key-Derivation-Metadata":     `{"kdf":"client-side-test"}`,
	})
	if status != http.StatusCreated {
		t.Fatalf("backup status=%d body=%s", status, response)
	}

	status, response = doJSON(t, handler, http.MethodGet, "/api/v1/account/export", token, nil)
	if status != http.StatusOK {
		t.Fatalf("export status=%d body=%s", status, response)
	}
	if !bytes.Contains(response, []byte(`"messages"`)) {
		t.Fatalf("export missing encrypted message envelopes: %s", response)
	}

	status, response = doJSON(t, handler, http.MethodDelete, "/api/v1/account", token, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete account status=%d body=%s", status, response)
	}
	status, _ = doJSON(t, handler, http.MethodGet, "/api/v1/conversations", token, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("deleted account token status=%d want %d", status, http.StatusUnauthorized)
	}
}

func newTestHandlerWithOwner(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "private-messenger.db")
	application, err := app.New(context.Background(), config.Config{
		Addr:         ":0",
		DataDir:      dir,
		DatabasePath: dbPath,
		StoragePath:  filepath.Join(dir, "blobs"),
		InstanceName: "Test Messenger",
	}, nil)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	handler := application.Handler()
	status, response := doJSON(t, handler, http.MethodPost, "/api/v1/setup/owner", "", map[string]interface{}{
		"instance_name":      "Test Messenger",
		"username":           "owner",
		"password":           "owner-password-123",
		"device_name":        "owner phone",
		"device_key_package": base64.StdEncoding.EncodeToString([]byte("owner-key-package")),
	})
	if status != http.StatusCreated {
		t.Fatalf("setup owner status=%d body=%s", status, response)
	}
	var decoded struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	return handler, decoded.Token, dbPath
}

func createConversation(t *testing.T, handler http.Handler, token string) string {
	t.Helper()
	status, response := doJSON(t, handler, http.MethodPost, "/api/v1/conversations", token, map[string]interface{}{"kind": "group"})
	if status != http.StatusCreated {
		t.Fatalf("create conversation status=%d body=%s", status, response)
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	return decoded.ID
}

func createMessage(t *testing.T, handler http.Handler, token, conversationID, idempotencyKey string, ciphertext []byte) string {
	t.Helper()
	status, response := doJSON(t, handler, http.MethodPost, "/api/v1/messages/envelopes", token, map[string]interface{}{
		"conversation_id": conversationID,
		"idempotency_key": idempotencyKey,
		"ciphertext":      base64.StdEncoding.EncodeToString(ciphertext),
		"crypto_protocol": "mls-openmls-todo",
	})
	if status != http.StatusCreated {
		t.Fatalf("create message status=%d body=%s", status, response)
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatalf("decode message: %v", err)
	}
	return decoded.ID
}

func registerMember(t *testing.T, handler http.Handler, ownerToken, username string) string {
	t.Helper()
	status, response := doJSON(t, handler, http.MethodPost, "/api/v1/invites", ownerToken, map[string]interface{}{"max_uses": 1})
	if status != http.StatusCreated {
		t.Fatalf("create invite status=%d body=%s", status, response)
	}
	var invite struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response, &invite); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	status, response = doJSON(t, handler, http.MethodPost, "/api/v1/register", "", map[string]interface{}{
		"invite_code":        invite.Code,
		"username":           username,
		"password":           "member-password-123",
		"device_name":        username + " phone",
		"device_key_package": base64.StdEncoding.EncodeToString([]byte(username + "-key-package")),
	})
	if status != http.StatusCreated {
		t.Fatalf("register member status=%d body=%s", status, response)
	}
	var decoded struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	return decoded.Token
}

func doJSON(t *testing.T, handler http.Handler, method, path, token string, body interface{}) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if path == "/api/v1/setup/owner" {
		req.Header.Set("X-Private-Messenger-Setup", "1")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func doRaw(t *testing.T, handler http.Handler, method, path, token string, body []byte, headers map[string]string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}
