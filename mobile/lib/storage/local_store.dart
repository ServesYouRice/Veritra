import '../core/models.dart';

abstract class LocalStore {
  Future<void> saveSession(Session session);
  Future<Session?> loadSession();
  Future<void> clear();
}

class MemoryLocalStore implements LocalStore {
  Session? _session;

  @override
  Future<void> saveSession(Session session) async {
    _session = session;
  }

  @override
  Future<Session?> loadSession() async {
    return _session;
  }

  @override
  Future<void> clear() async {
    _session = null;
  }
}

