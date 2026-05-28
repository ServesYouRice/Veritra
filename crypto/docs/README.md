# Crypto Boundary

The selected production direction is MLS through OpenMLS. This Rust crate currently exposes only a fail-closed availability marker so mobile or server code cannot accidentally treat placeholder crypto as production-ready.

Before production message sending:

- add OpenMLS dependency after license review
- define FFI-safe device key package APIs
- add MLS test vectors
- add mobile secure key storage
- update `docs/crypto-research.md`
- update `THIRD_PARTY_NOTICES.md`

