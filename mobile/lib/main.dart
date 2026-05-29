import 'package:flutter/material.dart';

import 'core/app_state.dart';
import 'core/api_client.dart';
import 'crypto/crypto_service.dart';
import 'storage/local_store.dart';
import 'sync/sync_service.dart';
import 'ui/app_shell.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final state = AppState(
    apiClientFactory: (baseUrl) => ApiClient(baseUrl: baseUrl),
    cryptoService: UnavailableCryptoService(),
    localStore: SecureLocalStore(),
    syncServiceFactory: (baseUrl, token) => WebSocketSyncService(baseUrl: baseUrl, token: token),
  );
  // Best-effort: restore a previously stored session so the user doesn't have
  // to re-authenticate on every cold start. If the call fails (corrupt
  // keystore entry, missing secure storage on a new platform) we silently
  // fall through to the connect screen.
  await state.tryRestoreSession();
  runApp(PrivateMessengerApp(state: state));
}

class PrivateMessengerApp extends StatelessWidget {
  const PrivateMessengerApp({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: state,
      builder: (context, _) {
        return MaterialApp(
          title: 'Private Messenger',
          theme: ThemeData(
            colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xff126f7a)),
            useMaterial3: true,
          ),
          home: AppShell(state: state),
        );
      },
    );
  }
}

