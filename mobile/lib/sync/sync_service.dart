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
  bool _disposed = false;
  Future<void>? _connectLoop;

  @override
  Stream<Map<String, Object?>> get events => _controller.stream;

  @override
  Future<void> connect() {
    _connectLoop ??= _runConnectLoop();
    return Future<void>.value();
  }

  Future<void> _runConnectLoop() async {
    var delay = const Duration(seconds: 1);
    while (!_disposed) {
      try {
        await _connectOnce();
        delay = const Duration(seconds: 1);
      } catch (err, stackTrace) {
        if (!_disposed && !_controller.isClosed) {
          _controller.addError(err, stackTrace);
        }
      }
      if (!_disposed) {
        await Future<void>.delayed(delay);
        final nextSeconds = delay.inSeconds * 2;
        delay = Duration(seconds: nextSeconds > 30 ? 30 : nextSeconds);
      }
    }
  }

  Future<void> _connectOnce() async {
    final base = Uri.parse(baseUrl);
    final uri = base.replace(
      scheme: base.scheme == 'https' ? 'wss' : 'ws',
      path: '/api/v1/sync/ws',
    );
    // Send the token via the Authorization header so it never lands in URLs,
    // server access logs, or reverse-proxy logs.
    final socket = await WebSocket.connect(
      uri.toString(),
      headers: <String, dynamic>{'Authorization': 'Bearer $token'},
    ).timeout(const Duration(seconds: 15));
    socket.pingInterval = const Duration(seconds: 30);
    _socket = socket;
    final done = Completer<void>();
    socket.listen((data) {
      if (!_disposed && !_controller.isClosed && data is String) {
        _controller.add(Map<String, Object?>.from(jsonDecode(data) as Map));
      }
    }, onDone: () {
      if (!done.isCompleted) {
        done.complete();
      }
    }, onError: (Object err, StackTrace stackTrace) {
      if (!_disposed && !_controller.isClosed) {
        _controller.addError(err, stackTrace);
      }
      if (!done.isCompleted) {
        done.complete();
      }
    }, cancelOnError: true);
    await done.future;
  }

  @override
  void dispose() {
    _disposed = true;
    _socket?.close();
    _controller.close();
  }
}
