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
    final uri = Uri.parse(baseUrl).replace(
      scheme: Uri.parse(baseUrl).scheme == 'https' ? 'wss' : 'ws',
      path: '/api/v1/sync/ws',
      queryParameters: <String, String>{'token': token},
    );
    _socket = await WebSocket.connect(uri.toString());
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

