import 'dart:async';

import 'package:flutter/foundation.dart';

import '../crypto/crypto_service.dart';
import '../storage/local_store.dart';
import '../sync/sync_service.dart';
import 'api_client.dart';
import 'models.dart';

typedef ApiClientFactory = ApiClient Function(String baseUrl);
typedef SyncServiceFactory = SyncService Function(String baseUrl, String token);

class AppState extends ChangeNotifier {
  AppState({
    required this.apiClientFactory,
    required this.cryptoService,
    required this.localStore,
    required this.syncServiceFactory,
  });

  final ApiClientFactory apiClientFactory;
  final CryptoService cryptoService;
  final LocalStore localStore;
  final SyncServiceFactory syncServiceFactory;

  Session? session;
  ApiClient? api;
  SyncService? sync;
  StreamSubscription<Map<String, Object?>>? _syncSubscription;
  List<Conversation> conversations = <Conversation>[];
  List<Device> devices = <Device>[];
  Map<String, List<ReceivedMessageEnvelope>> messagesByConversation =
      <String, List<ReceivedMessageEnvelope>>{};
  String? selectedConversationId;
  DeviceLink? activeDeviceLink;
  DeviceLinkClaim? pendingDeviceLinkClaim;
  String? error;
  bool busy = false;
  bool _catchingUpSync = false;
  int _lastSyncEventId = 0;

  bool get connected => session != null;
  Conversation? get selectedConversation =>
      conversations.where((c) => c.id == selectedConversationId).firstOrNull;
  List<ReceivedMessageEnvelope> get selectedMessages {
    final id = selectedConversationId;
    if (id == null) {
      return const <ReceivedMessageEnvelope>[];
    }
    return messagesByConversation[id] ?? const <ReceivedMessageEnvelope>[];
  }

  Future<void> connect(String baseUrl) async {
    api = apiClientFactory(baseUrl);
    await api!.setupStatus();
    error = null;
    notifyListeners();
  }

  /// Best-effort hydration of a previously-stored session on cold start.
  /// Failures are swallowed: a stale or unreadable session simply lands the
  /// user on the connect screen rather than crashing the app.
  Future<void> tryRestoreSession() async {
    try {
      final restored = await localStore.loadSession();
      if (restored == null) {
        return;
      }
      if (restored.token.isEmpty) {
        return;
      }
      session = restored;
      api = apiClientFactory(restored.baseUrl);
      _lastSyncEventId = await localStore.loadSyncCursor();
      await refreshConversations();
      await refreshDevices();
      _startSync();
      notifyListeners();
    } catch (_) {
      // Runtime state falls back to the connect screen; the cached device ID
      // stays available for password login on an already-linked device.
      session = null;
      api = null;
      devices = <Device>[];
      messagesByConversation = <String, List<ReceivedMessageEnvelope>>{};
      _lastSyncEventId = 0;
      await localStore.saveSyncCursor(0);
    }
  }

  Future<void> createOwner(
      String baseUrl, String username, String password) async {
    await _run(() async {
      api = apiClientFactory(baseUrl);
      session = await api!.createOwner(
        username: username,
        password: password,
        deviceName: 'Mobile device',
        deviceKeyPackage: await cryptoService.createDeviceKeyPackage(),
      );
      await localStore.saveSession(session!);
      _lastSyncEventId = 0;
      await localStore.saveSyncCursor(0);
      await refreshConversations();
      await refreshDevices();
      _startSync();
    });
  }

  Future<void> login(String baseUrl, String username, String password) async {
    await _run(() async {
      api = apiClientFactory(baseUrl);
      final localSession = await localStore.loadSession();
      final deviceId =
          localSession?.baseUrl == baseUrl ? localSession?.deviceId : null;
      if (deviceId == null || deviceId.isEmpty) {
        throw StateError(
            'Password login requires this device to be linked first.');
      }
      session = await api!.login(
        username: username,
        password: password,
        deviceId: deviceId,
      );
      await localStore.saveSession(session!);
      _lastSyncEventId = 0;
      await localStore.saveSyncCursor(0);
      await refreshConversations();
      await refreshDevices();
      _startSync();
    });
  }

  Future<void> refreshConversations() async {
    await _refreshConversations(notify: true);
  }

  Future<void> refreshDevices() async {
    final current = session;
    final client = api;
    if (current == null || client == null) {
      return;
    }
    devices = await client.devices(current.token);
    notifyListeners();
  }

  Future<void> _refreshConversations({required bool notify}) async {
    final current = session;
    final client = api;
    if (current == null || client == null) {
      return;
    }
    conversations = await client.conversations(current.token);
    if (notify) {
      notifyListeners();
    }
  }

  Future<void> refreshSelectedMessages({bool notify = true}) async {
    final current = session;
    final client = api;
    final conversationId = selectedConversationId;
    if (current == null || client == null || conversationId == null) {
      return;
    }
    final messages = await client.listMessages(current.token, conversationId);
    messagesByConversation = <String, List<ReceivedMessageEnvelope>>{
      ...messagesByConversation,
      conversationId: messages,
    };
    if (notify) {
      notifyListeners();
    }
  }

  Future<void> createGroup() async {
    await _run(() async {
      final current = session;
      final client = api;
      if (current == null || client == null) {
        return;
      }
      final conversation =
          await client.createConversation(current.token, 'group');
      conversations = <Conversation>[conversation, ...conversations];
      selectedConversationId = conversation.id;
      messagesByConversation[conversation.id] = <ReceivedMessageEnvelope>[];
    });
  }

  Future<void> sendMessage(String plaintext) async {
    await _run(() async {
      final current = session;
      final client = api;
      final conversation = selectedConversation;
      if (current == null || client == null || conversation == null) {
        return;
      }
      final encrypted = await cryptoService.encrypt(conversation.id, plaintext);
      await client.sendEnvelope(current.token, encrypted);
      await refreshSelectedMessages(notify: false);
    });
  }

  Future<void> createDeviceLink() async {
    await _run(() async {
      final current = session;
      final client = api;
      if (current == null || client == null) {
        return;
      }
      activeDeviceLink = await client.createDeviceLink(current.token);
    });
  }

  Future<void> approveActiveDeviceLink(String verificationCode) async {
    await _run(() async {
      final current = session;
      final client = api;
      final link = activeDeviceLink;
      if (current == null || client == null || link == null) {
        return;
      }
      activeDeviceLink = await client.approveDeviceLink(
        current.token,
        link.id,
        verificationCode,
      );
    });
  }

  Future<void> refreshActiveDeviceLink() async {
    await _run(() async {
      final current = session;
      final client = api;
      final link = activeDeviceLink;
      if (current == null || client == null || link == null) {
        return;
      }
      final refreshed = await client.deviceLink(current.token, link.id);
      activeDeviceLink = DeviceLink(
        id: refreshed.id,
        state: refreshed.state,
        verificationCode: refreshed.verificationCode,
        expiresAt: refreshed.expiresAt,
        code: link.code ?? refreshed.code,
        linkUri: link.linkUri ?? refreshed.linkUri,
        claimedDeviceName: refreshed.claimedDeviceName,
        approvedDeviceId: refreshed.approvedDeviceId,
      );
    });
  }

  Future<void> claimDeviceLink(String baseUrl, String code) async {
    await _run(() async {
      api = apiClientFactory(baseUrl);
      pendingDeviceLinkClaim = await api!.claimDeviceLink(
        code: code,
        deviceName: 'Linked mobile device',
        deviceKeyPackage: await cryptoService.createDeviceKeyPackage(),
      );
    });
  }

  Future<void> completeDeviceLinkClaim() async {
    await _run(() async {
      final client = api;
      final claim = pendingDeviceLinkClaim;
      if (client == null || claim == null) {
        return;
      }
      final linkedSession = await client.completeDeviceLinkClaim(
        claim.deviceLink.id,
        claim.claimToken,
      );
      if (linkedSession == null) {
        return;
      }
      session = linkedSession;
      pendingDeviceLinkClaim = null;
      await localStore.saveSession(linkedSession);
      _lastSyncEventId = 0;
      await localStore.saveSyncCursor(0);
      await refreshConversations();
      await refreshDevices();
      _startSync();
    });
  }

  Future<void> logout() async {
    await _run(() async {
      final current = session;
      final client = api;
      if (current != null && client != null) {
        await client.logout(current.token);
      }
      await _clearLocalSession(preserveDeviceIdentity: true);
    });
  }

  Future<void> logoutOtherDevices() async {
    await _run(() async {
      final current = session;
      final client = api;
      if (current == null || client == null) {
        return;
      }
      await client.logoutAll(current.token);
      await refreshDevices();
    });
  }

  Future<void> revokeDevice(String deviceId) async {
    await _run(() async {
      final current = session;
      final client = api;
      if (current == null || client == null) {
        return;
      }
      await client.revokeDevice(current.token, deviceId);
      if (deviceId == current.deviceId) {
        await _clearLocalSession();
      } else {
        await refreshDevices();
      }
    });
  }

  void selectConversation(String id) {
    selectedConversationId = id;
    notifyListeners();
    unawaited(refreshSelectedMessages());
  }

  void _startSync() {
    final current = session;
    if (current == null) {
      return;
    }
    unawaited(_syncSubscription?.cancel());
    sync?.dispose();
    sync = syncServiceFactory(current.baseUrl, current.token);
    _syncSubscription = sync!.events.listen(
      (_) => unawaited(_catchUpSyncEvents()),
      onError: (_) => unawaited(_catchUpSyncEvents()),
    );
    unawaited(_catchUpSyncEvents());
    unawaited(sync!.connect());
  }

  Future<void> _catchUpSyncEvents() async {
    if (_catchingUpSync) {
      return;
    }
    final current = session;
    final client = api;
    if (current == null || client == null) {
      return;
    }
    _catchingUpSync = true;
    try {
      final events =
          await client.syncEvents(current.token, after: _lastSyncEventId);
      var refreshConversationsNeeded = false;
      var refreshSelectedMessagesNeeded = false;
      final selectedId = selectedConversationId;
      for (final event in events) {
        if (event.id > _lastSyncEventId) {
          _lastSyncEventId = event.id;
        }
        if (event.conversationId != null) {
          refreshConversationsNeeded = true;
          if (event.conversationId == selectedId) {
            refreshSelectedMessagesNeeded = true;
          }
        } else if (event.type.startsWith('device.')) {
          await refreshDevices();
          refreshConversationsNeeded = true;
        } else if (event.type.startsWith('conversation.')) {
          refreshConversationsNeeded = true;
        }
      }
      if (events.isNotEmpty) {
        await localStore.saveSyncCursor(_lastSyncEventId);
      }
      if (refreshConversationsNeeded) {
        await _refreshConversations(notify: false);
      }
      if (refreshSelectedMessagesNeeded) {
        await refreshSelectedMessages(notify: false);
      }
      if (events.isNotEmpty) {
        notifyListeners();
      }
    } catch (err) {
      error = err.toString();
      notifyListeners();
    } finally {
      _catchingUpSync = false;
    }
  }

  Future<void> _clearLocalSession({bool preserveDeviceIdentity = false}) async {
    final current = session;
    unawaited(_syncSubscription?.cancel());
    _syncSubscription = null;
    sync?.dispose();
    sync = null;
    if (preserveDeviceIdentity &&
        current != null &&
        current.deviceId != null &&
        current.deviceId!.isNotEmpty) {
      await localStore.saveSession(Session(
        baseUrl: current.baseUrl,
        token: '',
        accountId: current.accountId,
        deviceId: current.deviceId,
      ));
      await localStore.saveSyncCursor(0);
    } else {
      await localStore.clear();
    }
    session = null;
    api = null;
    conversations = <Conversation>[];
    devices = <Device>[];
    messagesByConversation = <String, List<ReceivedMessageEnvelope>>{};
    selectedConversationId = null;
    activeDeviceLink = null;
    pendingDeviceLinkClaim = null;
    _lastSyncEventId = 0;
  }

  Future<void> _run(Future<void> Function() body) async {
    busy = true;
    error = null;
    notifyListeners();
    try {
      await body();
    } catch (err) {
      error = err.toString();
    } finally {
      busy = false;
      notifyListeners();
    }
  }

  @override
  void dispose() {
    unawaited(_syncSubscription?.cancel());
    sync?.dispose();
    super.dispose();
  }
}

extension FirstOrNull<T> on Iterable<T> {
  T? get firstOrNull {
    final iterator = this.iterator;
    if (!iterator.moveNext()) {
      return null;
    }
    return iterator.current;
  }
}
