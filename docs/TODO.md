# TODO

- Integrate OpenMLS through Rust bindings and Flutter FFI.
- Implement production encrypted local mobile storage for key material and the message cache. (Session secrets already use `flutter_secure_storage`; private keys and decrypted message storage are the remaining gap.)
- Add QR scanning/rendering and production key-continuity checks to the current manual device-link UX.
- Add encrypted backup creation and restore UX on mobile.
- Add APNs/FCM/UnifiedPush provider implementations.
- Add client-side local message search.
- Add attachment upload from Flutter with client-side encryption.
- Add WebRTC media or LiveKit integration after call E2EE review.
- Add PostgreSQL and S3 adapters only when needed.
- Run full dependency license scan before release.
