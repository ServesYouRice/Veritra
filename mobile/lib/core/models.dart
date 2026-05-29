import 'dart:convert';

class Conversation {
  Conversation({required this.id, required this.kind, this.title});

  final String id;
  final String kind;
  final String? title;

  factory Conversation.fromJson(Map<String, Object?> json) {
    return Conversation(
      id: json['id'] as String,
      kind: json['kind'] as String,
      title: json['title'] as String?,
    );
  }
}

class MessageEnvelope {
  MessageEnvelope({
    required this.conversationId,
    required this.idempotencyKey,
    required this.ciphertext,
    required this.cryptoProtocol,
    this.cryptoMetadata = const <String, Object?>{},
    this.attachmentRefs = const <Object?>[],
    this.replyToId,
    this.threadRootId,
  });

  final String conversationId;
  final String idempotencyKey;
  final List<int> ciphertext;
  final String cryptoProtocol;
  final Map<String, Object?> cryptoMetadata;
  final List<Object?> attachmentRefs;
  final String? replyToId;
  final String? threadRootId;

  Map<String, Object?> toJson() {
    return <String, Object?>{
      'conversation_id': conversationId,
      'idempotency_key': idempotencyKey,
      'ciphertext': base64Encode(ciphertext),
      'crypto_protocol': cryptoProtocol,
      'crypto_metadata': cryptoMetadata,
      'attachment_refs': attachmentRefs,
      if (replyToId != null) 'reply_to_id': replyToId,
      if (threadRootId != null) 'thread_root_id': threadRootId,
    };
  }
}

class MetadataSearchResult {
  MetadataSearchResult(
      {required this.type, required this.id, required this.label});

  final String type;
  final String id;
  final String label;

  factory MetadataSearchResult.fromJson(Map<String, Object?> json) {
    return MetadataSearchResult(
      type: json['type'] as String,
      id: json['id'] as String,
      label: json['label'] as String,
    );
  }
}

class DeviceLink {
  DeviceLink({
    required this.id,
    required this.state,
    required this.verificationCode,
    required this.expiresAt,
    this.code,
    this.linkUri,
    this.claimedDeviceName,
    this.approvedDeviceId,
  });

  final String id;
  final String state;
  final String verificationCode;
  final DateTime expiresAt;
  final String? code;
  final String? linkUri;
  final String? claimedDeviceName;
  final String? approvedDeviceId;

  factory DeviceLink.fromJson(Map<String, Object?> json) {
    return DeviceLink(
      id: json['id'] as String,
      state: json['state'] as String,
      verificationCode: json['verification_code'] as String,
      expiresAt: DateTime.parse(json['expires_at'] as String),
      code: json['code'] as String?,
      linkUri: json['link_uri'] as String?,
      claimedDeviceName: json['claimed_device_name'] as String?,
      approvedDeviceId: json['approved_device_id'] as String?,
    );
  }
}

class DeviceLinkClaim {
  DeviceLinkClaim({required this.deviceLink, required this.claimToken});

  final DeviceLink deviceLink;
  final String claimToken;
}

class Session {
  const Session({required this.baseUrl, required this.token});

  final String baseUrl;
  final String token;
}
