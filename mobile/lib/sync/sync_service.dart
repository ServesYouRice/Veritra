import 'dart:async';
import 'dart:convert';
import 'dart:io';

abstract class SyncService {
  Stream<Map<String, Object?>> get events;
  Future<void> connect();
  void dispose();
}

class WebSocketSyncService implements SyncService {
  WebSocketSyncService({required this.baseUrl, required this.token});

  final String baseUrl;
  final String token;
  final _controller = StreamController<Map<String, Object?>>.broadcast();
  WebSocket? _socket;

  @override
  Stream<Map<String, Object?>> get events => _controller.stream;

  @override
  Future<void> connect() async {
    final base = Uri.parse(baseUrl);
    final uri = base.replace(
      scheme: base.scheme == 'https' ? 'wss' : 'ws',
      path: '/api/v1/sync/ws',
    );
    // Send the token via the Authorization header so it never lands in URLs,
    // server access logs, or reverse-proxy logs.
    _socket = await WebSocket.connect(
      uri.toString(),
      headers: <String, dynamic>{'Authorization': 'Bearer $token'},
    );
    _socket!.listen((data) {
      if (data is String) {
        _controller.add(jsonDecode(data) as Map<String, Object?>);
      }
    }, onDone: () {}, onError: _controller.addError);
  }

  @override
  void dispose() {
    _socket?.close();
    _controller.close();
  }
}

