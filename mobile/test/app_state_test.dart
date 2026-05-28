import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:private_messenger/core/app_state.dart';
import 'package:private_messenger/core/models.dart';
import 'package:private_messenger/crypto/crypto_service.dart';
import 'package:private_messenger/storage/local_store.dart';
import 'package:private_messenger/sync/sync_service.dart';

void main() {
  test('message envelope serializes ciphertext without plaintext body field',
      () {
    final envelope = MessageEnvelope(
      conversationId: 'conv_1',
      idempotencyKey: 'key_1',
      ciphertext: <int>[1, 2, 3],
      cryptoProtocol: 'mls-openmls-todo',
    );
    final json = envelope.toJson();
    expect(json.containsKey('ciphertext'), isTrue);
    expect(json.containsKey('body'), isFalse);
    expect(json.containsKey('text'), isFalse);
  });

  test('metadata search result parses non-message metadata only', () {
    final result = MetadataSearchResult.fromJson(<String, Object?>{
      'type': 'community',
      'id': 'comm_1',
      'label': 'Family',
    });
    expect(result.type, 'community');
    expect(result.label, 'Family');
  });

  test('app state can store session through local abstraction', () async {
    final localStore = MemoryLocalStore();
    final state = AppState(
      apiClientFactory: (_) => throw UnimplementedError(),
      cryptoService: TestOnlyCryptoService(),
      localStore: localStore,
      syncServiceFactory: (_, __) => FakeSyncService(),
    );
    await localStore.saveSession(
        const Session(baseUrl: 'http://localhost:8080', token: 'token'));
    expect((await state.localStore.loadSession())?.token, 'token');
  });
}

class FakeSyncService implements SyncService {
  final _controller = StreamController<Map<String, Object?>>.broadcast();

  @override
  Stream<Map<String, Object?>> get events => _controller.stream;

  @override
  Future<void> connect() async {}

  @override
  void dispose() {
    _controller.close();
  }
}
