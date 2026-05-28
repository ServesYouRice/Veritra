# Recovery and Backups

No admin can recover user plaintext.

The intended model is:

- client encrypts backups before upload
- server stores `BackupBlob` ciphertext and metadata only
- recovery phrase/key stays with the user
- QR device linking is preferred for adding devices
- no server-side plaintext keys

The current MVP includes a server API for uploading client-encrypted backup blobs. Production encrypted backup UX, restore flows, and cryptographic implementation remain TODO.
