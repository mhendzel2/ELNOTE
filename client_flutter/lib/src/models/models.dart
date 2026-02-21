class AuthSession {
  AuthSession({
    required this.baseUrl,
    required this.accessToken,
    required this.refreshToken,
    required this.accessTokenExpiresAt,
  });

  final String baseUrl;
  final String accessToken;
  final String refreshToken;
  final DateTime accessTokenExpiresAt;
}

class ExperimentRecord {
  ExperimentRecord({
    required this.localId,
    required this.serverId,
    required this.title,
    required this.status,
    required this.effectiveBody,
    required this.originalEntryServerId,
    required this.effectiveEntryServerId,
    required this.updatedAt,
  });

  final String localId;
  final String? serverId;
  final String title;
  final String status;
  final String effectiveBody;
  final String? originalEntryServerId;
  final String? effectiveEntryServerId;
  final DateTime updatedAt;
}

class EntryRecord {
  EntryRecord({
    required this.localId,
    required this.experimentLocalId,
    required this.serverId,
    required this.entryType,
    required this.supersedesServerId,
    required this.body,
    required this.createdAt,
  });

  final String localId;
  final String experimentLocalId;
  final String? serverId;
  final String entryType;
  final String? supersedesServerId;
  final String body;
  final DateTime createdAt;
}

class CommentRecord {
  CommentRecord({
    required this.localId,
    required this.experimentLocalId,
    required this.serverId,
    required this.body,
    required this.createdAt,
  });

  final String localId;
  final String experimentLocalId;
  final String? serverId;
  final String body;
  final DateTime createdAt;
}

class ProposalRecord {
  ProposalRecord({
    required this.localId,
    required this.sourceExperimentLocalId,
    required this.serverId,
    required this.title,
    required this.body,
    required this.createdAt,
  });

  final String localId;
  final String sourceExperimentLocalId;
  final String? serverId;
  final String title;
  final String body;
  final DateTime createdAt;
}

class OutboxItem {
  OutboxItem({
    required this.id,
    required this.mutationType,
    required this.payloadJson,
    required this.status,
    required this.attempts,
  });

  final int id;
  final String mutationType;
  final String payloadJson;
  final String status;
  final int attempts;
}

class SyncEvent {
  SyncEvent({
    required this.cursor,
    required this.eventType,
    required this.aggregateType,
    required this.aggregateId,
    required this.payload,
  });

  final int cursor;
  final String eventType;
  final String aggregateType;
  final String? aggregateId;
  final Map<String, dynamic> payload;

  factory SyncEvent.fromJson(Map<String, dynamic> json) {
    return SyncEvent(
      cursor: (json['cursor'] as num).toInt(),
      eventType: json['eventType'] as String,
      aggregateType: json['aggregateType'] as String,
      aggregateId: json['aggregateId'] as String?,
      payload: (json['payload'] as Map<String, dynamic>? ?? <String, dynamic>{}),
    );
  }
}

class ConflictArtifact {
  ConflictArtifact({
    required this.conflictArtifactId,
    required this.experimentId,
    required this.clientBaseEntryId,
    required this.serverLatestEntryId,
    required this.createdAt,
  });

  final String conflictArtifactId;
  final String experimentId;
  final String? clientBaseEntryId;
  final String? serverLatestEntryId;
  final DateTime createdAt;

  factory ConflictArtifact.fromJson(Map<String, dynamic> json) {
    return ConflictArtifact(
      conflictArtifactId: json['conflictArtifactId'] as String,
      experimentId: json['experimentId'] as String,
      clientBaseEntryId: json['clientBaseEntryId'] as String?,
      serverLatestEntryId: json['serverLatestEntryId'] as String?,
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}
