# Calls

The MVP server includes call signaling scaffolding. Media is not production-ready.

## Direction

- 1:1 calls first.
- Pion is preferred for embedded Go signaling/media experiments.
- LiveKit is the reference for a production SFU if group calls require it.
- Small group calls should not make the default deployment heavy.

Call E2EE status must be documented before enabling production media.

