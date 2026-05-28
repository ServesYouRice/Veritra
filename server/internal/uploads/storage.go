package uploads

import (
	"context"
	"io"
	"os"
)

type Store interface {
	PutEncryptedBlob(ctx context.Context, r io.Reader) (storageKey string, sha256Hex string, size int64, err error)
	Open(storageKey string) (*os.File, error)
}
