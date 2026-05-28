package webrtc

import (
	"context"
	"encoding/json"

	"private-messenger/server/internal/domain"
)

type SignalingService interface {
	CreateCall(ctx context.Context, conversationID, accountID string, metadata json.RawMessage) (domain.CallSession, error)
	SendSignal(ctx context.Context, callID, accountID string, payload json.RawMessage) error
}

type SignalEvent struct {
	CallID    string          `json:"call_id"`
	AccountID string          `json:"account_id"`
	Payload   json.RawMessage `json:"payload"`
}
