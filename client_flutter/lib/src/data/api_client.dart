import 'dart:convert';

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
  }) async {
    final response = await _post(
      '/v1/experiments',
      body: {'title': title, 'originalBody': originalBody},
    );
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
