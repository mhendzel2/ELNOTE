import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import 'api_client.dart';
import 'local_database.dart';

class SyncService {
  SyncService({required this.db, required this.api});

  final LocalDatabase db;
  final ApiClient api;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _wsSubscription;
  Timer? _reconnectTimer;
  bool _isSyncing = false;

  Future<void> replayOutbox() async {
    final items = await db.listPendingOutbox(limit: 200);
    for (final item in items) {
      final payload = jsonDecode(item.payloadJson) as Map<String, dynamic>;

      try {
        switch (item.mutationType) {
          case 'create_experiment':
            await _replayCreateExperiment(item.id, payload);
            break;
          case 'add_addendum':
            await _replayAddAddendum(item.id, payload);
            break;
          case 'add_comment':
            await _replayAddComment(item.id, payload);
            break;
          case 'create_proposal':
            await _replayCreateProposal(item.id, payload);
            break;
          case 'create_protocol':
            await _replayCreateProtocol(item.id, payload);
            break;
          case 'publish_protocol_version':
            await _replayPublishProtocolVersion(item.id, payload);
            break;
          case 'sign_experiment':
            await _replaySignExperiment(item.id, payload);
            break;
          case 'add_tag':
            await _replayAddTag(item.id, payload);
            break;
          case 'record_deviation':
            await _replayRecordDeviation(item.id, payload);
            break;
          case 'create_template':
            await _replayCreateTemplate(item.id, payload);
            break;
          case 'create_chart_config':
            await _replayCreateChartConfig(item.id, payload);
            break;
          default:
            await db.markOutboxError(item.id, 'unknown mutation type: ${item.mutationType}');
            break;
        }
      } on ApiException catch (e) {
        if (e.statusCode == 409 && e.body?['conflictArtifactId'] != null) {
          await db.insertConflictArtifact(e.body!);
          await db.markOutboxConflict(item.id, e.message);
        } else {
          await db.markOutboxError(item.id, e.message);
        }
      } catch (e) {
        await db.markOutboxError(item.id, e.toString());
      }
    }
  }

  Future<void> pullOnce() async {
    final cursor = await db.getCursor();
    final payload = await api.pullSync(cursor: cursor, limit: 100);
    final events = (payload['events'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();

    for (final event in events) {
      final body = (event['payload'] as Map<String, dynamic>? ?? <String, dynamic>{});
      final experimentId = body['experimentId'] as String? ?? body['sourceExperimentId'] as String?;
      if (experimentId != null && experimentId.isNotEmpty) {
        await _hydrateExperiment(experimentId);
      }

      if ((event['eventType'] as String?) == 'conflict.stale_addendum') {
        final conflicts = await api.listConflicts(limit: 100);
        for (final conflict in conflicts) {
          await db.insertConflictArtifact(conflict);
        }
      }
    }

    final nextCursor = (payload['cursor'] as num?)?.toInt() ?? cursor;
    await db.setCursor(nextCursor);
  }

  Future<void> syncNow() async {
    if (_isSyncing) {
      return;
    }
    _isSyncing = true;
    try {
      await replayOutbox();
      await pullOnce();
    } finally {
      _isSyncing = false;
    }
  }

  Future<void> startWebSocket() async {
    if (_wsSubscription != null) {
      return;
    }

    final token = api.accessToken;
    if (token == null || token.isEmpty) {
      return;
    }

    final cursor = await db.getCursor();
    final uri = Uri.parse('${api.websocketUrl}/v1/sync/ws').replace(
      queryParameters: <String, String>{
        'cursor': '$cursor',
        // Browsers cannot set arbitrary WS headers, so pass token in query.
        'access_token': token,
      },
    );

    _channel = WebSocketChannel.connect(uri);

    _wsSubscription = _channel!.stream.listen(
      (dynamic message) async {
        await _onWebSocketMessage(message);
      },
      onError: (_) => _scheduleReconnect(),
      onDone: _scheduleReconnect,
      cancelOnError: true,
    );
  }

  Future<void> stopWebSocket() async {
    _reconnectTimer?.cancel();
    _reconnectTimer = null;

    await _wsSubscription?.cancel();
    _wsSubscription = null;

    await _channel?.sink.close();
    _channel = null;
  }

  Future<void> dispose() async {
    await stopWebSocket();
  }

  Future<void> _onWebSocketMessage(dynamic message) async {
    Map<String, dynamic> json;
    if (message is String) {
      json = jsonDecode(message) as Map<String, dynamic>;
    } else {
      return;
    }

    final type = json['type'] as String?;
    if (type == 'events') {
      await syncNow();
    }
  }

  void _scheduleReconnect() {
    _wsSubscription = null;
    _channel = null;

    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(const Duration(seconds: 3), () async {
      await startWebSocket();
    });
  }

  Future<void> _replayCreateExperiment(int outboxId, Map<String, dynamic> payload) async {
    final localExperimentId = payload['experimentLocalId'] as String;
    final title = payload['title'] as String;
    final originalBody = payload['originalBody'] as String;

    final response = await api.createExperiment(title: title, originalBody: originalBody);

    await db.attachServerExperimentIdentity(
      localExperimentId: localExperimentId,
      serverExperimentId: response['experimentId'] as String,
      originalEntryServerId: response['originalEntryId'] as String,
      effectiveEntryServerId: response['originalEntryId'] as String,
      effectiveBody: originalBody,
      status: response['status'] as String? ?? 'draft',
    );

    await db.markOutboxDone(outboxId);
  }

  Future<void> _replayAddAddendum(int outboxId, Map<String, dynamic> payload) async {
    final localExperimentId = payload['experimentLocalId'] as String;
    final serverExperimentId = await db.getServerExperimentId(localExperimentId);
    if (serverExperimentId == null || serverExperimentId.isEmpty) {
      return;
    }

    final response = await api.createAddendum(
      experimentId: serverExperimentId,
      body: payload['body'] as String,
      baseEntryId: payload['baseEntryId'] as String?,
    );

    await _hydrateExperiment(serverExperimentId);
    if (response['entryId'] != null) {
      await db.markOutboxDone(outboxId);
      return;
    }

    await db.markOutboxError(outboxId, 'unexpected addendum response');
  }

  Future<void> _replayAddComment(int outboxId, Map<String, dynamic> payload) async {
    final localExperimentId = payload['experimentLocalId'] as String;
    final serverExperimentId = await db.getServerExperimentId(localExperimentId);
    if (serverExperimentId == null || serverExperimentId.isEmpty) {
      return;
    }

    await api.addComment(
      experimentId: serverExperimentId,
      body: payload['body'] as String,
    );

    await _hydrateExperiment(serverExperimentId);
    await db.markOutboxDone(outboxId);
  }

  Future<void> _replayCreateProposal(int outboxId, Map<String, dynamic> payload) async {
    final localExperimentId = payload['sourceExperimentLocalId'] as String;
    final serverExperimentId = await db.getServerExperimentId(localExperimentId);
    if (serverExperimentId == null || serverExperimentId.isEmpty) {
      return;
    }

    await api.createProposal(
      sourceExperimentId: serverExperimentId,
      title: payload['title'] as String,
      body: payload['body'] as String,
    );

    await _hydrateExperiment(serverExperimentId);
    await db.markOutboxDone(outboxId);
  }

  Future<void> _hydrateExperiment(String serverExperimentId) async {
    final effective = await api.getExperiment(serverExperimentId);
    final localId = await db.upsertExperimentFromServer(effective);

    final history = await api.getHistory(serverExperimentId);
    await db.replaceEntriesForExperiment(
      experimentLocalId: localId,
      entries: history,
    );

    try {
      final comments = await api.listComments(serverExperimentId);
      await db.replaceCommentsForExperiment(
        experimentLocalId: localId,
        comments: comments,
      );
    } on ApiException {
      // Not all roles/scopes can fetch comments for all experiments.
    }

    try {
      final proposals = await api.listProposals(serverExperimentId);
      await db.replaceProposalsForExperiment(
        experimentLocalId: localId,
        proposals: proposals,
      );
    } on ApiException {
      // Not all roles/scopes can fetch proposals for all experiments.
    }

    // Hydrate signatures
    try {
      final sigs = await api.listSignatures(serverExperimentId);
      await db.replaceSignaturesForExperiment(
        experimentServerId: serverExperimentId,
        signatures: sigs,
      );
    } on ApiException {
      // May not have permission.
    }

    // Hydrate tags
    try {
      final tags = await api.listTags(serverExperimentId);
      await db.replaceTagsForExperiment(
        experimentServerId: serverExperimentId,
        tags: tags,
      );
    } on ApiException {
      // May not have permission.
    }
  }

  // ---------------------------------------------------------------------------
  // Replay: Protocols
  // ---------------------------------------------------------------------------

  Future<void> _replayCreateProtocol(int outboxId, Map<String, dynamic> payload) async {
    await api.createProtocol(
      title: payload['title'] as String,
      description: payload['description'] as String? ?? '',
      initialBody: payload['initialBody'] as String? ?? payload['description'] as String? ?? '',
    );
    await db.markOutboxDone(outboxId);
  }

  Future<void> _replayPublishProtocolVersion(int outboxId, Map<String, dynamic> payload) async {
    await api.publishProtocolVersion(
      protocolId: payload['protocolId'] as String,
      body: payload['body'] as String,
      changeSummary: payload['changeSummary'] as String? ?? payload['changeLog'] as String? ?? '',
    );
    await db.markOutboxDone(outboxId);
  }

  // ---------------------------------------------------------------------------
  // Replay: Signatures
  // ---------------------------------------------------------------------------

  Future<void> _replaySignExperiment(int outboxId, Map<String, dynamic> payload) async {
    final serverExperimentId = payload['experimentServerId'] as String;
    await api.signExperiment(
      experimentId: serverExperimentId,
      signatureType: payload['signatureType'] as String?,
      meaning: payload['meaning'] as String?,
      password: payload['password'] as String,
    );
    await _hydrateExperiment(serverExperimentId);
    await db.markOutboxDone(outboxId);
  }

  // ---------------------------------------------------------------------------
  // Replay: Tags
  // ---------------------------------------------------------------------------

  Future<void> _replayAddTag(int outboxId, Map<String, dynamic> payload) async {
    final serverExperimentId = payload['experimentServerId'] as String;
    await api.addTag(
      experimentId: serverExperimentId,
      tag: payload['name'] as String,
    );
    await _hydrateExperiment(serverExperimentId);
    await db.markOutboxDone(outboxId);
  }

  // ---------------------------------------------------------------------------
  // Replay: Deviations
  // ---------------------------------------------------------------------------

  Future<void> _replayRecordDeviation(int outboxId, Map<String, dynamic> payload) async {
    final serverExperimentId = payload['experimentServerId'] as String;
    await api.recordDeviation(
      experimentId: serverExperimentId,
      experimentEntryId: payload['experimentEntryId'] as String?,
      deviationType: payload['deviationType'] as String?,
      rationale: payload['rationale'] as String?,
      protocolId: payload['protocolId'] as String?,
      description: payload['description'] as String?,
      severity: payload['severity'] as String?,
    );
    await _hydrateExperiment(serverExperimentId);
    await db.markOutboxDone(outboxId);
  }

  // ---------------------------------------------------------------------------
  // Replay: Templates
  // ---------------------------------------------------------------------------

  Future<void> _replayCreateTemplate(int outboxId, Map<String, dynamic> payload) async {
    await api.createTemplate(
      title: payload['title'] as String,
      description: payload['description'] as String? ?? '',
      bodyTemplate: payload['bodyTemplate'] as String? ?? '',
      sections: (payload['sections'] as List<dynamic>?)
              ?.map((e) => e as Map<String, dynamic>)
              .toList() ??
          [],
    );
    await db.markOutboxDone(outboxId);
  }

  // ---------------------------------------------------------------------------
  // Replay: Chart config
  // ---------------------------------------------------------------------------

  Future<void> _replayCreateChartConfig(int outboxId, Map<String, dynamic> payload) async {
    await api.createChartConfig(
      experimentId: payload['experimentServerId'] as String,
      dataExtractId: payload['dataExtractId'] as String,
      chartType: payload['chartType'] as String? ?? 'line',
      title: payload['title'] as String? ?? '',
      xColumn: payload['xColumn'] as String? ?? '',
      yColumns: (payload['yColumns'] as List<dynamic>?)
              ?.map((e) => e as String)
              .toList() ??
          [],
    );
    await db.markOutboxDone(outboxId);
  }

  // ---------------------------------------------------------------------------
  // Bulk sync helpers for new entities
  // ---------------------------------------------------------------------------

  Future<void> refreshProtocols() async {
    final protocols = await api.listProtocols();
    for (final p in protocols) {
      await db.upsertProtocol(p);
    }
  }

  Future<void> refreshNotifications() async {
    final notifications = await api.listNotifications();
    await db.replaceNotifications(notifications);
  }

  Future<void> refreshTemplates() async {
    final templates = await api.listTemplates();
    await db.replaceTemplates(templates);
  }
}
