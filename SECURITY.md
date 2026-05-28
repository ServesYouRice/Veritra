# Security Policy

Private Messenger is currently an MVP foundation and has not received a production security audit.

## Supported Versions

Only the main development branch is supported during the initial MVP phase.

## Reporting Vulnerabilities

Please report vulnerabilities privately to the project maintainers once a public security contact is established. Do not file public issues for vulnerabilities involving authentication, cryptography, message persistence, push payloads, or server-side data exposure.

## Current Security Caveats

- Production E2EE is not complete. MLS/OpenMLS integration is the selected direction, but the current server only enforces ciphertext-only persistence boundaries.
- Push providers, encrypted backup, and WebRTC media are scaffolded or documented until production integrations are complete.
- Self-hosters should run behind HTTPS for any non-LAN deployment.

