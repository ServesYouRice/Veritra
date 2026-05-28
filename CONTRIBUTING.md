# Contributing

Contributions are welcome if they preserve the privacy and security model.

## Requirements

- License contributions under AGPL-3.0-or-later.
- Do not add telemetry or analytics.
- Do not add server-side plaintext message handling.
- Do not import code without license review.
- Update `THIRD_PARTY_NOTICES.md` for new dependencies.
- Add or update tests for auth, devices, permissions, encrypted envelope persistence, and migration changes.

## Development

Use Dockerized scripts by default:

```sh
./scripts/test.sh
./scripts/lint.sh
```

If local toolchains are installed, use the native commands documented in `AGENTS.md`.

