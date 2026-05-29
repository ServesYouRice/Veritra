import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:private_messenger/core/api_client.dart';
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

  test('device link parses QR verification metadata', () {
    final link = DeviceLink.fromJson(<String, Object?>{
      'id': 'dlink_1',
      'state': 'pending',
      'verification_code': '123456',
      'expires_at': '2026-05-29T12:00:00Z',
      'code': 'PAIRCODE',
      'link_uri': 'veritra://device-link?code=PAIRCODE',
    });
    expect(link.code, 'PAIRCODE');
    expect(link.verificationCode, '123456');
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

  test('app state drives device link claim through approval', () async {
    final localStore = MemoryLocalStore();
    final api = FakeDeviceLinkApiClient();
    final state = AppState(
      apiClientFactory: (_) => api,
      cryptoService: TestOnlyCryptoService(),
      localStore: localStore,
      syncServiceFactory: (_, __) => FakeSyncService(),
    );

    await state.claimDeviceLink('http://localhost:8080', 'PAIRCODE');
    expect(state.pendingDeviceLinkClaim?.deviceLink.verificationCode, '654321');
    expect(state.session, isNull);

    await state.completeDeviceLinkClaim();
    expect(state.session?.token, 'linked-token');
    expect((await localStore.loadSession())?.token, 'linked-token');
  });

  test('app state can create and approve a device link', () async {
    final api = FakeDeviceLinkApiClient();
    final state = AppState(
      apiClientFactory: (_) => api,
      cryptoService: TestOnlyCryptoService(),
      localStore: MemoryLocalStore(),
      syncServiceFactory: (_, __) => FakeSyncService(),
    )
      ..api = api
      ..session = const Session(
        baseUrl: 'http://localhost:8080',
        token: 'owner-token',
      );

    await state.createDeviceLink();
    expect(state.activeDeviceLink?.code, 'PAIRCODE');

    await state.refreshActiveDeviceLink();
    expect(state.activeDeviceLink?.claimedDeviceName, 'linked tablet');
    expect(state.activeDeviceLink?.code, 'PAIRCODE');

    await state.approveActiveDeviceLink();
    expect(state.activeDeviceLink?.state, 'approved');
    expect(state.activeDeviceLink?.approvedDeviceId, 'dev_linked');
  });
}

class FakeDeviceLinkApiClient extends ApiClient {
  FakeDeviceLinkApiClient() : super(baseUrl: 'http://localhost:8080');

  @override
  Future<DeviceLink> createDeviceLink(String token) async {
    return _link(state: 'pending', code: 'PAIRCODE');
  }

  @override
  Future<DeviceLink> deviceLink(String token, String linkId) async {
    return _link(state: 'claimed', claimedDeviceName: 'linked tablet');
  }

  @override
  Future<DeviceLinkClaim> claimDeviceLink({
    required String code,
    required String deviceName,
    required List<int> deviceKeyPackage,
    List<int> signingKey = const <int>[],
  }) async {
    return DeviceLinkClaim(
      deviceLink: _link(state: 'claimed'),
      claimToken: 'claim-token',
    );
  }

  @override
  Future<DeviceLink> approveDeviceLink(String token, String linkId) async {
    return _link(
      state: 'approved',
      code: 'PAIRCODE',
      approvedDeviceId: 'dev_linked',
    );
  }

  @override
  Future<Session?> completeDeviceLinkClaim(
      String linkId, String claimToken) async {
    return const Session(
      baseUrl: 'http://localhost:8080',
      token: 'linked-token',
    );
  }

  @override
  Future<List<Conversation>> conversations(String token) async {
    return <Conversation>[];
  }

  DeviceLink _link({
    required String state,
    String? code,
    String? approvedDeviceId,
    String? claimedDeviceName,
  }) {
    return DeviceLink(
      id: 'dlink_1',
      state: state,
      verificationCode: '654321',
      expiresAt: DateTime.parse('2026-05-29T12:00:00Z'),
      code: code,
      linkUri: code == null ? null : 'veritra://device-link?code=$code',
      claimedDeviceName: claimedDeviceName,
      approvedDeviceId: approvedDeviceId,
    );
  }
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
