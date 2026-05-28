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
  List<Conversation> conversations = <Conversation>[];
  String? selectedConversationId;
  String? error;
  bool busy = false;

  bool get connected => session != null;
  Conversation? get selectedConversation => conversations.where((c) => c.id == selectedConversationId).firstOrNull;

  Future<void> connect(String baseUrl) async {
    api = apiClientFactory(baseUrl);
    await api!.setupStatus();
    error = null;
    notifyListeners();
  }

  Future<void> createOwner(String baseUrl, String username, String password) async {
    await _run(() async {
      api = apiClientFactory(baseUrl);
      session = await api!.createOwner(
        username: username,
        password: password,
        deviceName: 'Mobile device',
        deviceKeyPackage: await cryptoService.createDeviceKeyPackage(),
      );
      await localStore.saveSession(session!);
      await refreshConversations();
      _startSync();
    });
  }

  Future<void> login(String baseUrl, String username, String password) async {
    await _run(() async {
      api = apiClientFactory(baseUrl);
      session = await api!.login(username: username, password: password);
      await localStore.saveSession(session!);
      await refreshConversations();
      _startSync();
    });
  }

  Future<void> refreshConversations() async {
    final current = session;
    final client = api;
    if (current == null || client == null) {
      return;
    }
    conversations = await client.conversations(current.token);
    notifyListeners();
  }

  Future<void> createGroup() async {
    await _run(() async {
      final current = session;
      final client = api;
      if (current == null || client == null) {
        return;
      }
      final conversation = await client.createConversation(current.token, 'group');
      conversations = <Conversation>[conversation, ...conversations];
      selectedConversationId = conversation.id;
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
    });
  }

  void selectConversation(String id) {
    selectedConversationId = id;
    notifyListeners();
  }

  void _startSync() {
    final current = session;
    if (current == null) {
      return;
    }
    sync?.dispose();
    sync = syncServiceFactory(current.baseUrl, current.token);
    sync!.events.listen((_) => refreshConversations());
    sync!.connect();
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

