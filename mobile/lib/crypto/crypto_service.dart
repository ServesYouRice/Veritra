import '../core/models.dart';

abstract class CryptoService {
  Future<List<int>> createDeviceKeyPackage();
  Future<MessageEnvelope> encrypt(String conversationId, String plaintext);
}

class UnavailableCryptoService implements CryptoService {
  @override
  Future<List<int>> createDeviceKeyPackage() async {
    throw StateError('Production MLS/OpenMLS device key package creation is not integrated');
  }

  @override
  Future<MessageEnvelope> encrypt(String conversationId, String plaintext) async {
    throw StateError('Production MLS/OpenMLS encryption is not integrated');
  }
}

class TestOnlyCryptoService implements CryptoService {
  @override
  Future<List<int>> createDeviceKeyPackage() async {
    return 'TEST_ONLY_DEVICE_KEY_PACKAGE'.codeUnits;
  }

  @override
  Future<MessageEnvelope> encrypt(String conversationId, String plaintext) async {
    return MessageEnvelope(
      conversationId: conversationId,
      idempotencyKey: DateTime.now().microsecondsSinceEpoch.toString(),
      ciphertext: 'TEST_ONLY_CIPHERTEXT_LEN:${plaintext.length}'.codeUnits,
      cryptoProtocol: 'test-only-not-production',
      cryptoMetadata: const <String, Object?>{'warning': 'not-production-crypto'},
    );
  }
}
