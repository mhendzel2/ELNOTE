import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import '../models/models.dart';

class ApiException implements Exception {
  ApiException(this.statusCode, this.message, {this.body});

  final int statusCode;
  final String message;
  final Map<String, dynamic>? body;

  @override
  String toString() => 'ApiException($statusCode): $message';
}

class ApiClient {
  ApiClient({required this.baseUrl, this.accessToken});

  final String baseUrl;
  String? accessToken;

  String get websocketUrl {
    if (baseUrl.startsWith('https://')) {
      return baseUrl.replaceFirst('https://', 'wss://');
    }
    return baseUrl.replaceFirst('http://', 'ws://');
  }

  Future<AuthSession> login({
    required String email,
    required String password,
    required String deviceName,
  }) async {
    final response = await _post(
      '/v1/auth/login',
      body: {
        'email': email,
        'password': password,
        'deviceName': deviceName,
      },
      withAuth: false,
    );

    final json = _decode(response);
    return AuthSession(
      baseUrl: baseUrl,
      accessToken: json['accessToken'] as String,
      refreshToken: json['refreshToken'] as String,
      accessTokenExpiresAt: DateTime.parse(json['accessTokenExpiresAt'] as String),
    );
  }

  Future<Map<String, dynamic>> createExperiment({
    required String title,
    required String originalBody,
    String? projectId,
  }) async {
    final body = <String, dynamic>{
      'title': title,
      'originalBody': originalBody,
    };
    if (projectId != null) body['projectId'] = projectId;
    final response = await _post('/v1/experiments', body: body);
    return _decode(response);
  }

  Future<Map<String, dynamic>> createAddendum({
    required String experimentId,
    required String body,
    required String? baseEntryId,
  }) async {
    final response = await _post(
      '/v1/experiments/$experimentId/addendums',
      body: {
        'body': body,
        if (baseEntryId != null && baseEntryId.isNotEmpty) 'baseEntryId': baseEntryId,
      },
    );
    return _decode(response);
  }

  Future<Map<String, dynamic>> getExperiment(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId');
    return _decode(response);
  }

  Future<Map<String, dynamic>> markCompleted(String experimentId) async {
    final response = await _post('/v1/experiments/$experimentId/complete');
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> getHistory(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/history');
    final json = _decode(response);
    return (json['entries'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<List<Map<String, dynamic>>> listComments(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/comments');
    final json = _decode(response);
    return (json['comments'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> addComment({
    required String experimentId,
    required String body,
  }) async {
    final response = await _post(
      '/v1/experiments/$experimentId/comments',
      body: {'body': body},
    );
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listProposals(String sourceExperimentId) async {
    final response = await _get('/v1/proposals?sourceExperimentId=$sourceExperimentId');
    final json = _decode(response);
    return (json['proposals'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> createProposal({
    required String sourceExperimentId,
    required String title,
    required String body,
  }) async {
    final response = await _post(
      '/v1/proposals',
      body: {
        'sourceExperimentId': sourceExperimentId,
        'title': title,
        'body': body,
      },
    );
    return _decode(response);
  }

  Future<Map<String, dynamic>> pullSync({
    required int cursor,
    int limit = 100,
  }) async {
    final response = await _get('/v1/sync/pull?cursor=$cursor&limit=$limit');
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listConflicts({int limit = 100}) async {
    final response = await _get('/v1/sync/conflicts?limit=$limit');
    final json = _decode(response);
    return (json['conflicts'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  // -------------------------------------------------------------------------
  // Protocols
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> createProtocol({
    required String title,
    required String description,
  }) async {
    final response = await _post('/v1/protocols', body: {
      'title': title,
      'description': description,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listProtocols({String? status}) async {
    final qs = status != null && status.isNotEmpty ? '?status=$status' : '';
    final response = await _get('/v1/protocols$qs');
    final json = _decode(response);
    return (json['protocols'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> getProtocol(String protocolId) async {
    final response = await _get('/v1/protocols/$protocolId');
    return _decode(response);
  }

  Future<Map<String, dynamic>> publishProtocolVersion({
    required String protocolId,
    required String body,
    required String changeLog,
  }) async {
    final response = await _post('/v1/protocols/$protocolId/publish', body: {
      'body': body,
      'changeLog': changeLog,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listProtocolVersions(String protocolId) async {
    final response = await _get('/v1/protocols/$protocolId/versions');
    final json = _decode(response);
    return (json['versions'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<void> updateProtocolStatus({
    required String protocolId,
    required String status,
  }) async {
    await _post('/v1/protocols/$protocolId/status', body: {'status': status});
  }

  Future<Map<String, dynamic>> linkProtocol({
    required String experimentId,
    required String protocolId,
    required int versionNum,
  }) async {
    final response = await _post('/v1/experiments/$experimentId/protocols', body: {
      'protocolId': protocolId,
      'versionNum': versionNum,
    });
    return _decode(response);
  }

  Future<Map<String, dynamic>> recordDeviation({
    required String experimentId,
    required String protocolId,
    required String description,
    required String severity,
  }) async {
    final response = await _post('/v1/experiments/$experimentId/deviations', body: {
      'protocolId': protocolId,
      'description': description,
      'severity': severity,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listDeviations(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/deviations');
    final json = _decode(response);
    return (json['deviations'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  // -------------------------------------------------------------------------
  // Search
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> search({
    required String query,
    String? tag,
  }) async {
    var qs = '?q=${Uri.encodeQueryComponent(query)}';
    if (tag != null && tag.isNotEmpty) {
      qs += '&tag=${Uri.encodeQueryComponent(tag)}';
    }
    final response = await _get('/v1/search$qs');
    return _decode(response);
  }

  // -------------------------------------------------------------------------
  // Users
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> createUser({
    required String email,
    required String password,
    required String role,
  }) async {
    final response = await _post('/v1/users', body: {
      'email': email,
      'password': password,
      'role': role,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listUsers() async {
    final response = await _get('/v1/users');
    final json = _decode(response);
    return (json['users'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> getUser(String userId) async {
    final response = await _get('/v1/users/$userId');
    return _decode(response);
  }

  Future<Map<String, dynamic>> updateUser({
    required String userId,
    required String role,
  }) async {
    final response = await _put('/v1/users/$userId', body: {
      'role': role,
    });
    return _decode(response);
  }

  Future<void> deleteUser(String userId) async {
    await _request('DELETE', '/v1/users/$userId', withAuth: true);
  }

  Future<void> changePassword({
    required String userId,
    required String currentPassword,
    required String newPassword,
  }) async {
    await _post('/v1/users/$userId/change-password', body: {
      'currentPassword': currentPassword,
      'newPassword': newPassword,
    });
  }

  Future<Map<String, dynamic>> resetLabAdmin() async {
    final response = await _post('/v1/admin/reset-default');
    return _decode(response);
  }

  // -------------------------------------------------------------------------
  // Signatures
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> signExperiment({
    required String experimentId,
    required String password,
    required String meaning,
  }) async {
    final response = await _post('/v1/signatures', body: {
      'experimentId': experimentId,
      'password': password,
      'meaning': meaning,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listSignatures(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/signatures');
    final json = _decode(response);
    return (json['signatures'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> verifySignatures(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/signatures/verify');
    return _decode(response);
  }

  // -------------------------------------------------------------------------
  // Notifications
  // -------------------------------------------------------------------------

  Future<List<Map<String, dynamic>>> listNotifications({
    bool unreadOnly = false,
    int limit = 50,
  }) async {
    final response = await _get(
      '/v1/notifications?unreadOnly=$unreadOnly&limit=$limit',
    );
    final json = _decode(response);
    return (json['notifications'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<void> markNotificationRead(String notificationId) async {
    await _post('/v1/notifications/$notificationId/read');
  }

  Future<void> markAllNotificationsRead() async {
    await _post('/v1/notifications/read-all');
  }

  // -------------------------------------------------------------------------
  // Data Visualization
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> parseCSV({
    required String attachmentId,
    required String experimentId,
    required String csvData,
  }) async {
    final response = await _post('/v1/data/parse-csv', body: {
      'attachmentId': attachmentId,
      'experimentId': experimentId,
      'csvData': csvData,
    });
    return _decode(response);
  }

  Future<Map<String, dynamic>> getDataExtract(String extractId) async {
    final response = await _get('/v1/data/extracts/$extractId');
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listDataExtracts(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/data-extracts');
    final json = _decode(response);
    return (json['dataExtracts'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> createChart({
    required String experimentId,
    required String dataExtractId,
    required String chartType,
    required String title,
    required String xColumn,
    required List<String> yColumns,
    Map<String, dynamic>? options,
  }) async {
    final response = await _post('/v1/charts', body: {
      'experimentId': experimentId,
      'dataExtractId': dataExtractId,
      'chartType': chartType,
      'title': title,
      'xColumn': xColumn,
      'yColumns': yColumns,
      if (options != null) 'options': options,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listCharts(String experimentId) async {
    final response = await _get('/v1/charts?experimentId=$experimentId');
    final json = _decode(response);
    return (json['charts'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  // -------------------------------------------------------------------------
  // Templates
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> createTemplate({
    required String title,
    required String description,
    required String bodyTemplate,
    List<Map<String, dynamic>>? sections,
    String? protocolId,
    List<String>? tags,
  }) async {
    final response = await _post('/v1/templates', body: {
      'title': title,
      'description': description,
      'bodyTemplate': bodyTemplate,
      if (sections != null) 'sections': sections,
      if (protocolId != null) 'protocolId': protocolId,
      if (tags != null) 'tags': tags,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listTemplates() async {
    final response = await _get('/v1/templates');
    final json = _decode(response);
    return (json['templates'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> getTemplate(String templateId) async {
    final response = await _get('/v1/templates/$templateId');
    return _decode(response);
  }

  Future<Map<String, dynamic>> updateTemplate({
    required String templateId,
    required String title,
    required String description,
    required String bodyTemplate,
    List<Map<String, dynamic>>? sections,
    String? protocolId,
    List<String>? tags,
  }) async {
    final response = await _put('/v1/templates/$templateId', body: {
      'title': title,
      'description': description,
      'bodyTemplate': bodyTemplate,
      if (sections != null) 'sections': sections,
      if (protocolId != null) 'protocolId': protocolId,
      if (tags != null) 'tags': tags,
    });
    return _decode(response);
  }

  Future<void> deleteTemplate(String templateId) async {
    await _request('DELETE', '/v1/templates/$templateId', withAuth: true);
  }

  Future<Map<String, dynamic>> cloneExperiment({
    required String sourceExperimentId,
    required String newTitle,
  }) async {
    final response = await _post('/v1/experiments/clone', body: {
      'sourceExperimentId': sourceExperimentId,
      'newTitle': newTitle,
    });
    return _decode(response);
  }

  Future<Map<String, dynamic>> createFromTemplate({
    required String templateId,
    required String title,
    String? body,
  }) async {
    final response = await _post('/v1/experiments/from-template', body: {
      'templateId': templateId,
      'title': title,
      if (body != null) 'body': body,
    });
    return _decode(response);
  }

  // -------------------------------------------------------------------------
  // Tags
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> addTag({
    required String experimentId,
    required String tag,
  }) async {
    final response = await _post('/v1/experiments/$experimentId/tags', body: {
      'tag': tag,
    });
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listTags(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/tags');
    final json = _decode(response);
    return (json['tags'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  // -------------------------------------------------------------------------
  // Previews / Thumbnails
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> getAttachmentPreview(String attachmentId) async {
    final response = await _get('/v1/attachments/$attachmentId/preview');
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listExperimentPreviews(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/previews');
    final json = _decode(response);
    return (json['previews'] as List<dynamic>? ?? <dynamic>[])
        .cast<Map<String, dynamic>>();
  }

  // -------------------------------------------------------------------------
  // Data Visualisation / Chart Configs
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> createChartConfig({
    required String experimentId,
    required String dataExtractId,
    required String chartType,
    String title = '',
    String xColumn = '',
    List<String> yColumns = const [],
  }) async {
    final response = await _post('/v1/charts', body: {
      'experimentId': experimentId,
      'dataExtractId': dataExtractId,
      'chartType': chartType,
      'title': title,
      'xColumn': xColumn,
      'yColumns': yColumns,
    });
    return _decode(response);
  }

  // =========================================================================
  // Ops / Admin
  // =========================================================================

  Future<Map<String, dynamic>> getOpsDashboard() async {
    final response = await _get('/v1/ops/dashboard');
    return _decode(response);
  }

  Future<Map<String, dynamic>> verifyAuditChain() async {
    final response = await _get('/v1/ops/audit/verify');
    return _decode(response);
  }

  Future<Map<String, dynamic>> forensicExport(String experimentId) async {
    final response = await _get('/v1/ops/forensic/export?experimentId=${Uri.encodeQueryComponent(experimentId)}');
    return _decode(response);
  }

  // =========================================================================
  // Attachments
  // =========================================================================

  Future<List<Map<String, dynamic>>> listExperimentAttachments(String experimentId) async {
    final response = await _get('/v1/experiments/$experimentId/attachments');
    final data = _decode(response);
    return (data['attachments'] as List).cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> initiateAttachment({
    required String experimentId,
    required String objectKey,
    required int sizeBytes,
    required String mimeType,
  }) async {
    final response = await _post('/v1/attachments/initiate', body: {
      'experimentId': experimentId,
      'objectKey': objectKey,
      'sizeBytes': sizeBytes,
      'mimeType': mimeType,
    });
    return _decode(response);
  }

  Future<Map<String, dynamic>> completeAttachment({
    required String attachmentId,
    required String checksum,
    required int sizeBytes,
  }) async {
    final response = await _post('/v1/attachments/$attachmentId/complete', body: {
      'checksum': checksum,
      'sizeBytes': sizeBytes,
    });
    return _decode(response);
  }

  Future<Map<String, dynamic>> downloadAttachment(String attachmentId) async {
    final response = await _get('/v1/attachments/$attachmentId/download');
    return _decode(response);
  }

  // =========================================================================
  // Reagents — mutable lab inventory CRUD
  // =========================================================================

  // --- Storage ---
  Future<List<ReagentStorage>> listStorage() async {
    final response = await _get('/v1/reagents/storage');
    final data = _decode(response);
    return (data['storage'] as List)
        .map((e) => ReagentStorage.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentStorage> createStorage(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/storage', body: body);
    return ReagentStorage.fromJson(_decode(response));
  }

  Future<void> updateStorage(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/storage/$id', body: body);
  }

  Future<void> deleteStorage(int id) async {
    await _request('DELETE', '/v1/reagents/storage/$id', withAuth: true);
  }

  // --- Boxes ---
  Future<List<ReagentBox>> listBoxes({String q = ''}) async {
    final path = q.isEmpty ? '/v1/reagents/boxes' : '/v1/reagents/boxes?q=${Uri.encodeQueryComponent(q)}';
    final response = await _get(path);
    final data = _decode(response);
    return (data['boxes'] as List)
        .map((e) => ReagentBox.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentBox> createBox(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/boxes', body: body);
    return ReagentBox.fromJson(_decode(response));
  }

  Future<void> updateBox(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/boxes/$id', body: body);
  }

  Future<void> deleteBox(int id) async {
    await _request('DELETE', '/v1/reagents/boxes/$id', withAuth: true);
  }

  // --- Antibodies ---
  Future<List<ReagentAntibody>> listAntibodies({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/antibodies$qs');
    final data = _decode(response);
    return (data['antibodies'] as List)
        .map((e) => ReagentAntibody.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentAntibody> createAntibody(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/antibodies', body: body);
    return ReagentAntibody.fromJson(_decode(response));
  }

  Future<void> updateAntibody(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/antibodies/$id', body: body);
  }

  Future<void> deleteAntibody(int id) async {
    await _request('DELETE', '/v1/reagents/antibodies/$id', withAuth: true);
  }

  // --- Cell Lines ---
  Future<List<ReagentCellLine>> listCellLines({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/cell-lines$qs');
    final data = _decode(response);
    return (data['cellLines'] as List)
        .map((e) => ReagentCellLine.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentCellLine> createCellLine(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/cell-lines', body: body);
    return ReagentCellLine.fromJson(_decode(response));
  }

  Future<void> updateCellLine(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/cell-lines/$id', body: body);
  }

  Future<void> deleteCellLine(int id) async {
    await _request('DELETE', '/v1/reagents/cell-lines/$id', withAuth: true);
  }

  // --- Viruses ---
  Future<List<ReagentVirus>> listViruses({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/viruses$qs');
    final data = _decode(response);
    return (data['viruses'] as List)
        .map((e) => ReagentVirus.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentVirus> createVirus(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/viruses', body: body);
    return ReagentVirus.fromJson(_decode(response));
  }

  Future<void> updateVirus(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/viruses/$id', body: body);
  }

  Future<void> deleteVirus(int id) async {
    await _request('DELETE', '/v1/reagents/viruses/$id', withAuth: true);
  }

  // --- DNA ---
  Future<List<ReagentDNA>> listDNA({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/dna$qs');
    final data = _decode(response);
    return (data['dna'] as List)
        .map((e) => ReagentDNA.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentDNA> createDNA(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/dna', body: body);
    return ReagentDNA.fromJson(_decode(response));
  }

  Future<void> updateDNA(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/dna/$id', body: body);
  }

  Future<void> deleteDNA(int id) async {
    await _request('DELETE', '/v1/reagents/dna/$id', withAuth: true);
  }

  // --- Oligos ---
  Future<List<ReagentOligo>> listOligos({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/oligos$qs');
    final data = _decode(response);
    return (data['oligos'] as List)
        .map((e) => ReagentOligo.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentOligo> createOligo(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/oligos', body: body);
    return ReagentOligo.fromJson(_decode(response));
  }

  Future<void> updateOligo(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/oligos/$id', body: body);
  }

  Future<void> deleteOligo(int id) async {
    await _request('DELETE', '/v1/reagents/oligos/$id', withAuth: true);
  }

  // --- Chemicals ---
  Future<List<ReagentChemical>> listChemicals({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/chemicals$qs');
    final data = _decode(response);
    return (data['chemicals'] as List)
        .map((e) => ReagentChemical.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentChemical> createChemical(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/chemicals', body: body);
    return ReagentChemical.fromJson(_decode(response));
  }

  Future<void> updateChemical(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/chemicals/$id', body: body);
  }

  Future<void> deleteChemical(int id) async {
    await _request('DELETE', '/v1/reagents/chemicals/$id', withAuth: true);
  }

  // --- Molecular ---
  Future<List<ReagentMolecular>> listMolecular({String q = '', bool depleted = false}) async {
    final params = <String>[];
    if (q.isNotEmpty) params.add('q=${Uri.encodeQueryComponent(q)}');
    if (depleted) params.add('depleted=true');
    final qs = params.isEmpty ? '' : '?${params.join('&')}';
    final response = await _get('/v1/reagents/molecular$qs');
    final data = _decode(response);
    return (data['molecular'] as List)
        .map((e) => ReagentMolecular.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ReagentMolecular> createMolecular(Map<String, dynamic> body) async {
    final response = await _post('/v1/reagents/molecular', body: body);
    return ReagentMolecular.fromJson(_decode(response));
  }

  Future<void> updateMolecular(int id, Map<String, dynamic> body) async {
    await _put('/v1/reagents/molecular/$id', body: body);
  }

  Future<void> deleteMolecular(int id) async {
    await _request('DELETE', '/v1/reagents/molecular/$id', withAuth: true);
  }

  // --- Cross-type search ---
  Future<List<ReagentSearchResult>> searchReagents(String q) async {
    final response = await _get('/v1/reagents/search?q=${Uri.encodeQueryComponent(q)}');
    final data = _decode(response);
    return (data['results'] as List)
        .map((e) => ReagentSearchResult.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// Bulk-import reagents from parsed CSV data.
  /// [reagentType] is one of: storage, boxes, antibodies, cell-lines, viruses,
  /// dna, oligos, chemicals, molecular.
  /// [items] is a list of maps matching the JSON field names for that type.
  /// Returns {imported: int, errors: [String]?}.
  Future<Map<String, dynamic>> bulkImportReagents(
    String reagentType,
    List<Map<String, dynamic>> items,
  ) async {
    final response = await _post(
      '/v1/reagents/$reagentType/import',
      body: {'items': items},
    );
    return _decode(response);
  }

  Future<Map<String, dynamic>> importAccessDatabase({
    required Uint8List fileBytes,
    required String fileName,
  }) async {
    final uri = Uri.parse('$baseUrl/v1/reagents/import-access');
    final request = http.MultipartRequest('POST', uri);
    request.headers['Accept'] = 'application/json';

    final token = accessToken;
    if (token == null || token.isEmpty) {
      throw ApiException(401, 'missing access token');
    }
    request.headers['Authorization'] = 'Bearer $token';

    request.files.add(
      http.MultipartFile.fromBytes(
        'file',
        fileBytes,
        filename: fileName,
      ),
    );

    final streamed = await request.send();
    final response = await http.Response.fromStream(streamed);
    if (response.statusCode >= 200 && response.statusCode < 300) {
      return _decode(response);
    }

    Map<String, dynamic>? errorBody;
    try {
      errorBody = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      errorBody = null;
    }
    throw ApiException(
      response.statusCode,
      errorBody?['error'] as String? ?? 'request failed',
      body: errorBody,
    );
  }

  // ── Projects ──────────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> createProject({
    required String title,
    String description = '',
  }) async {
    final response = await _post(
      '/v1/projects',
      body: {'title': title, 'description': description},
    );
    return _decode(response);
  }

  Future<List<Map<String, dynamic>>> listProjects() async {
    final response = await _get('/v1/projects');
    final json = _decode(response);
    return (json['projects'] as List).cast<Map<String, dynamic>>();
  }

  Future<Map<String, dynamic>> getProject(String projectId) async {
    final response = await _get('/v1/projects/$projectId');
    return _decode(response);
  }

  Future<void> updateProject({
    required String projectId,
    String? title,
    String? description,
    String? status,
  }) async {
    final body = <String, dynamic>{};
    if (title != null) body['title'] = title;
    if (description != null) body['description'] = description;
    if (status != null) body['status'] = status;
    await _put('/v1/projects/$projectId', body: body);
  }

  Future<void> deleteProject(String projectId) async {
    await _request('DELETE', '/v1/projects/$projectId', withAuth: true);
  }

  Future<List<Map<String, dynamic>>> listProjectExperiments(
    String projectId,
  ) async {
    final response = await _get('/v1/projects/$projectId/experiments');
    final json = _decode(response);
    return (json['experiments'] as List).cast<Map<String, dynamic>>();
  }

  Future<http.Response> _get(String path, {bool withAuth = true}) {
    return _request('GET', path, withAuth: withAuth);
  }

  Future<http.Response> _post(
    String path, {
    Map<String, dynamic>? body,
    bool withAuth = true,
  }) {
    return _request('POST', path, body: body, withAuth: withAuth);
  }

  Future<http.Response> _put(
    String path, {
    Map<String, dynamic>? body,
    bool withAuth = true,
  }) {
    return _request('PUT', path, body: body, withAuth: withAuth);
  }

  Future<http.Response> _request(
    String method,
    String path, {
    Map<String, dynamic>? body,
    required bool withAuth,
  }) async {
    final uri = Uri.parse('$baseUrl$path');
    final headers = <String, String>{
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };

    if (withAuth) {
      final token = accessToken;
      if (token == null || token.isEmpty) {
        throw ApiException(401, 'missing access token');
      }
      headers['Authorization'] = 'Bearer $token';
    }

    late final http.Response response;
    switch (method) {
      case 'GET':
        response = await http.get(uri, headers: headers);
        break;
      case 'DELETE':
        response = await http.delete(uri, headers: headers);
        break;
      case 'PUT':
        response = await http.put(
          uri,
          headers: headers,
          body: jsonEncode(body ?? <String, dynamic>{}),
        );
        break;
      default:
        response = await http.post(
          uri,
          headers: headers,
          body: jsonEncode(body ?? <String, dynamic>{}),
        );
        break;
    }

    if (response.statusCode >= 200 && response.statusCode < 300) {
      return response;
    }

    Map<String, dynamic>? errorBody;
    try {
      errorBody = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      errorBody = null;
    }

    throw ApiException(
      response.statusCode,
      errorBody?['error'] as String? ?? 'request failed',
      body: errorBody,
    );
  }

  Map<String, dynamic> _decode(http.Response response) {
    if (response.body.isEmpty) {
      return <String, dynamic>{};
    }
    return jsonDecode(response.body) as Map<String, dynamic>;
  }
}
