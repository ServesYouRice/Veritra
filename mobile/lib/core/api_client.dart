import 'dart:convert';
import 'dart:io';

import 'models.dart';

class ApiClient {
  ApiClient({required this.baseUrl, HttpClient? httpClient})
      : _httpClient = httpClient ?? HttpClient();

  final String baseUrl;
  final HttpClient _httpClient;

  Future<Map<String, Object?>> setupStatus() async {
    return _jsonRequest('GET', '/api/v1/setup/status');
  }

  Future<Session> createOwner({
    required String username,
    required String password,
    required String deviceName,
    required List<int> deviceKeyPackage,
    String instanceName = 'Private Messenger',
  }) async {
    final json = await _jsonRequest('POST', '/api/v1/setup/owner',
        body: <String, Object?>{
          'instance_name': instanceName,
          'username': username,
          'password': password,
          'device_name': deviceName,
          'device_key_package': base64Encode(deviceKeyPackage),
        },
        setupRequest: true);
    return Session(baseUrl: baseUrl, token: json['token'] as String);
  }

  Future<Session> login(
      {required String username, required String password}) async {
    final json = await _jsonRequest('POST', '/api/v1/auth/login',
        body: <String, Object?>{
          'username': username,
          'password': password,
        });
    return Session(baseUrl: baseUrl, token: json['token'] as String);
  }

  Future<List<Conversation>> conversations(String token) async {
    final json =
        await _jsonRequest('GET', '/api/v1/conversations', token: token);
    final rows = (json['conversations'] as List<Object?>? ?? const <Object?>[])
        .cast<Map<String, Object?>>();
    return rows.map(Conversation.fromJson).toList();
  }

  Future<Conversation> createConversation(String token, String kind) async {
    final json = await _jsonRequest('POST', '/api/v1/conversations',
        token: token,
        body: <String, Object?>{
          'kind': kind,
        });
    return Conversation.fromJson(json);
  }

  Future<void> sendEnvelope(String token, MessageEnvelope envelope) async {
    await _jsonRequest('POST', '/api/v1/messages/envelopes',
        token: token, body: envelope.toJson());
  }

  Future<void> sendReaction(
      String token, String messageId, List<int> reactionCiphertext) async {
    await _jsonRequest('POST', '/api/v1/messages/$messageId/reactions',
        token: token,
        body: <String, Object?>{
          'reaction_ciphertext': base64Encode(reactionCiphertext),
        });
  }

  Future<void> markRead(
      String token, String conversationId, String messageId) async {
    await _jsonRequest(
        'POST', '/api/v1/conversations/$conversationId/read-receipts',
        token: token,
        body: <String, Object?>{
          'message_id': messageId,
        });
  }

  Future<List<MetadataSearchResult>> searchMetadata(
      String token, String query) async {
    final json = await _jsonRequest(
        'GET', '/api/v1/search/metadata?q=${Uri.encodeQueryComponent(query)}',
        token: token);
    final rows = (json['results'] as List<Object?>? ?? const <Object?>[])
        .cast<Map<String, Object?>>();
    return rows.map(MetadataSearchResult.fromJson).toList();
  }

  Future<Map<String, Object?>> _jsonRequest(
    String method,
    String path, {
    String? token,
    Map<String, Object?>? body,
    bool setupRequest = false,
  }) async {
    final uri = Uri.parse(baseUrl).resolve(path);
    final request = await _httpClient.openUrl(method, uri);
    request.headers.contentType = ContentType.json;
    if (token != null) {
      request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
    }
    if (setupRequest) {
      request.headers.set('X-Private-Messenger-Setup', '1');
    }
    if (body != null) {
      request.write(jsonEncode(body));
    }
    final response = await request.close();
    final text = await utf8.decodeStream(response);
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ApiException(response.statusCode, text);
    }
    if (text.isEmpty) {
      return <String, Object?>{};
    }
    return jsonDecode(text) as Map<String, Object?>;
  }
}

class ApiException implements Exception {
  ApiException(this.statusCode, this.body);

  final int statusCode;
  final String body;

  @override
  String toString() => 'ApiException($statusCode)';
}
