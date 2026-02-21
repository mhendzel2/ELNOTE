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
    if (method == 'GET') {
      response = await http.get(uri, headers: headers);
    } else {
      response = await http.post(
        uri,
        headers: headers,
        body: jsonEncode(body ?? <String, dynamic>{}),
      );
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
