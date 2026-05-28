# Private Messenger Mobile

Flutter mobile shell for Android and iOS.

The current client includes screens, state boundaries, API serialization, sync and storage abstractions, and a crypto interface. Production message encryption is not implemented here; the default crypto service fails closed.

Local checks:

```sh
flutter test
```

Platform projects can be generated with Flutter once the SDK is available:

```sh
flutter create --platforms android,ios .
```

