package cryptoapi

import (
	"context"
	"errors"
)

var ErrProductionCryptoUnavailable = errors.New("production E2EE crypto is not integrated; MLS/OpenMLS bindings are required")

type DeviceKeyPackage struct {
	DeviceID   string
	AccountID  string
	KeyPackage []byte
	SigningKey []byte
}

type EnvelopeMetadata struct {
	Protocol string
	Metadata []byte
}

type ClientCrypto interface {
	EncryptEnvelope(ctx context.Context, conversationID string, plaintext []byte) ([]byte, EnvelopeMetadata, error)
	DecryptEnvelope(ctx context.Context, conversationID string, ciphertext []byte, metadata EnvelopeMetadata) ([]byte, error)
	CreateDeviceKeyPackage(ctx context.Context, accountID, deviceID string) (DeviceKeyPackage, error)
}

type UnavailableProductionCrypto struct{}

func (UnavailableProductionCrypto) EncryptEnvelope(context.Context, string, []byte) ([]byte, EnvelopeMetadata, error) {
	return nil, EnvelopeMetadata{}, ErrProductionCryptoUnavailable
}

func (UnavailableProductionCrypto) DecryptEnvelope(context.Context, string, []byte, EnvelopeMetadata) ([]byte, error) {
	return nil, ErrProductionCryptoUnavailable
}

func (UnavailableProductionCrypto) CreateDeviceKeyPackage(context.Context, string, string) (DeviceKeyPackage, error) {
	return DeviceKeyPackage{}, ErrProductionCryptoUnavailable
}
