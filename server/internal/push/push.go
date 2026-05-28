package push

import (
	"context"
	"errors"
)

var ErrNoProvider = errors.New("push provider is not configured")

type Notification struct {
	AccountID string
	DeviceID  string
	EventID   string
}

type Provider interface {
	SendEncryptedEventAvailable(ctx context.Context, notification Notification) error
}

type DisabledProvider struct{}

func (DisabledProvider) SendEncryptedEventAvailable(context.Context, Notification) error {
	return ErrNoProvider
}

func GenericPayload() map[string]string {
	return map[string]string{"event": "new_encrypted_event_available"}
}
