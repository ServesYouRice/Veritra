import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

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

/// SecureLocalStore persists the Session to the platform keystore
/// (Android EncryptedSharedPreferences via Keystore, iOS Keychain).
/// The session blob never lands on disk in plaintext.
class SecureLocalStore implements LocalStore {
  SecureLocalStore({FlutterSecureStorage? storage})
      : _storage = storage ??
            const FlutterSecureStorage(
              aOptions: AndroidOptions(
                encryptedSharedPreferences: true,
                resetOnError: true,
              ),
              iOptions: IOSOptions(
                accessibility: KeychainAccessibility.first_unlock_this_device,
              ),
            );

  static const _key = 'veritra.session.v1';
  final FlutterSecureStorage _storage;

  @override
  Future<void> saveSession(Session session) async {
    final encoded = jsonEncode(<String, String>{
      'base_url': session.baseUrl,
      'token': session.token,
    });
    await _storage.write(key: _key, value: encoded);
  }

  @override
  Future<Session?> loadSession() async {
    final raw = await _storage.read(key: _key);
    if (raw == null || raw.isEmpty) {
      return null;
    }
    try {
      final decoded = jsonDecode(raw) as Map<String, Object?>;
      final baseUrl = decoded['base_url'] as String?;
      final token = decoded['token'] as String?;
      if (baseUrl == null || token == null) {
        return null;
      }
      return Session(baseUrl: baseUrl, token: token);
    } catch (_) {
      // Stored payload was tampered with or corrupt — drop it and start over
      // rather than crashing the app on launch.
      await _storage.delete(key: _key);
      return null;
    }
  }

  @override
  Future<void> clear() async {
    await _storage.delete(key: _key);
  }
}

