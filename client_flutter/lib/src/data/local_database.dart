import 'dart:convert';

import 'package:path/path.dart';
import 'package:sqflite/sqflite.dart';
import 'package:uuid/uuid.dart';

import '../models/models.dart';

class LocalDatabase {
  static const _uuid = Uuid();

  Database? _db;

  Future<void> open() async {
    if (_db != null) {
      return;
    }

    final dbPath = await getDatabasesPath();
    final path = join(dbPath, 'elnote_local.db');

    _db = await openDatabase(
      path,
      version: 2,
      onCreate: (db, _) async {
        await db.execute('''
          CREATE TABLE local_experiments (
            local_id TEXT PRIMARY KEY,
            server_id TEXT UNIQUE,
            title TEXT NOT NULL,
            status TEXT NOT NULL,
            effective_body TEXT NOT NULL,
            original_entry_server_id TEXT,
            effective_entry_server_id TEXT,
            updated_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_entries (
            local_id TEXT PRIMARY KEY,
            experiment_local_id TEXT NOT NULL,
            server_id TEXT UNIQUE,
            entry_type TEXT NOT NULL,
            supersedes_server_id TEXT,
            body TEXT NOT NULL,
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_comments (
            local_id TEXT PRIMARY KEY,
            experiment_local_id TEXT NOT NULL,
            server_id TEXT UNIQUE,
            body TEXT NOT NULL,
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_proposals (
            local_id TEXT PRIMARY KEY,
            source_experiment_local_id TEXT NOT NULL,
            server_id TEXT UNIQUE,
            title TEXT NOT NULL,
            body TEXT NOT NULL,
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE outbox (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            mutation_type TEXT NOT NULL,
            payload_json TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'pending',
            attempts INTEGER NOT NULL DEFAULT 0,
            last_error TEXT,
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_conflicts (
            local_id TEXT PRIMARY KEY,
            server_conflict_artifact_id TEXT UNIQUE,
            experiment_server_id TEXT NOT NULL,
            client_base_entry_id TEXT,
            server_latest_entry_id TEXT,
            payload_json TEXT,
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE sync_state (
            id INTEGER PRIMARY KEY CHECK (id = 1),
            cursor INTEGER NOT NULL DEFAULT 0
          )
        ''');

        await db.insert('sync_state', {'id': 1, 'cursor': 0});

        // New tables for enhanced features
        await db.execute('''
          CREATE TABLE local_protocols (
            protocol_id TEXT PRIMARY KEY,
            creator_user_id TEXT NOT NULL,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT 'draft',
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_protocol_versions (
            protocol_id TEXT NOT NULL,
            version_num INTEGER NOT NULL,
            author_user_id TEXT NOT NULL,
            body TEXT NOT NULL,
            change_log TEXT NOT NULL DEFAULT '',
            published_at INTEGER NOT NULL,
            PRIMARY KEY (protocol_id, version_num)
          )
        ''');

        await db.execute('''
          CREATE TABLE local_signatures (
            signature_id TEXT PRIMARY KEY,
            experiment_server_id TEXT NOT NULL,
            signer_user_id TEXT NOT NULL,
            signer_email TEXT NOT NULL DEFAULT '',
            role TEXT NOT NULL DEFAULT 'author',
            meaning TEXT NOT NULL DEFAULT '',
            content_hash TEXT NOT NULL DEFAULT '',
            signed_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_notifications (
            notification_id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL,
            event_type TEXT NOT NULL,
            title TEXT NOT NULL,
            body TEXT NOT NULL DEFAULT '',
            experiment_id TEXT,
            is_read INTEGER NOT NULL DEFAULT 0,
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_tags (
            tag_id TEXT NOT NULL,
            experiment_server_id TEXT NOT NULL,
            name TEXT NOT NULL,
            PRIMARY KEY (tag_id, experiment_server_id)
          )
        ''');

        await db.execute('''
          CREATE TABLE local_templates (
            template_id TEXT PRIMARY KEY,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            body_template TEXT NOT NULL DEFAULT '',
            sections_json TEXT NOT NULL DEFAULT '[]',
            protocol_id TEXT,
            tags_json TEXT NOT NULL DEFAULT '[]',
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_data_extracts (
            data_extract_id TEXT PRIMARY KEY,
            attachment_id TEXT NOT NULL,
            experiment_server_id TEXT NOT NULL,
            column_headers_json TEXT NOT NULL DEFAULT '[]',
            row_count INTEGER NOT NULL DEFAULT 0,
            sample_rows_json TEXT NOT NULL DEFAULT '[]',
            parsed_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_chart_configs (
            chart_config_id TEXT PRIMARY KEY,
            experiment_server_id TEXT NOT NULL,
            data_extract_id TEXT NOT NULL,
            chart_type TEXT NOT NULL DEFAULT 'line',
            title TEXT NOT NULL DEFAULT '',
            x_column TEXT NOT NULL DEFAULT '',
            y_columns_json TEXT NOT NULL DEFAULT '[]',
            options_json TEXT NOT NULL DEFAULT '{}',
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_deviations (
            deviation_id TEXT PRIMARY KEY,
            experiment_server_id TEXT NOT NULL,
            protocol_id TEXT NOT NULL,
            reported_by TEXT NOT NULL,
            description TEXT NOT NULL,
            severity TEXT NOT NULL DEFAULT 'minor',
            created_at INTEGER NOT NULL
          )
        ''');

        await db.execute('''
          CREATE TABLE local_previews (
            preview_id TEXT PRIMARY KEY,
            attachment_id TEXT NOT NULL UNIQUE,
            preview_type TEXT NOT NULL DEFAULT 'thumbnail',
            mime_type TEXT NOT NULL DEFAULT 'image/png',
            width INTEGER NOT NULL DEFAULT 0,
            height INTEGER NOT NULL DEFAULT 0,
            data_base64 TEXT NOT NULL DEFAULT '',
            created_at INTEGER NOT NULL
          )
        ''');
      },
      onUpgrade: (db, oldVersion, newVersion) async {
        if (oldVersion < 2) {
          // Drop and recreate is simplest for dev; production would use proper migration
          final tables = ['local_protocols', 'local_protocol_versions',
            'local_signatures', 'local_notifications', 'local_tags',
            'local_templates', 'local_data_extracts', 'local_chart_configs',
            'local_deviations', 'local_previews'];
          for (final table in tables) {
            await db.execute('CREATE TABLE IF NOT EXISTS $table (id TEXT PRIMARY KEY)');
          }
          // Re-run full schema via delete + recreate approach
          await db.execute('DROP TABLE IF EXISTS local_protocols');
          await db.execute('DROP TABLE IF EXISTS local_protocol_versions');
          await db.execute('DROP TABLE IF EXISTS local_signatures');
          await db.execute('DROP TABLE IF EXISTS local_notifications');
          await db.execute('DROP TABLE IF EXISTS local_tags');
          await db.execute('DROP TABLE IF EXISTS local_templates');
          await db.execute('DROP TABLE IF EXISTS local_data_extracts');
          await db.execute('DROP TABLE IF EXISTS local_chart_configs');
          await db.execute('DROP TABLE IF EXISTS local_deviations');
          await db.execute('DROP TABLE IF EXISTS local_previews');

          await _createEnhancedTables(db);
        }
      },
    );

    await _db!.execute('''
      CREATE TABLE IF NOT EXISTS app_settings (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL
      )
    ''');
  }

  Database get _database {
    final db = _db;
    if (db == null) {
      throw StateError('database is not opened');
    }
    return db;
  }

  Future<void> close() async {
    final db = _db;
    _db = null;
    await db?.close();
  }

  Future<String> getOrCreateDeviceName() async {
    final db = _database;
    final rows = await db.query(
      'app_settings',
      columns: ['value'],
      where: 'key = ?',
      whereArgs: ['device_name'],
      limit: 1,
    );
    if (rows.isNotEmpty) {
      final value = rows.first['value'] as String?;
      if (value != null && value.trim().isNotEmpty) {
        return value;
      }
    }

    final deviceName = 'device-${_uuid.v4()}';
    await db.insert(
      'app_settings',
      {'key': 'device_name', 'value': deviceName},
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
    return deviceName;
  }

  Future<String> createLocalExperimentDraft({
    required String title,
    required String originalBody,
  }) async {
    final db = _database;
    final now = DateTime.now().millisecondsSinceEpoch;
    final experimentLocalId = _uuid.v4();

    await db.transaction((txn) async {
      await txn.insert('local_experiments', {
        'local_id': experimentLocalId,
        'server_id': null,
        'title': title.trim(),
        'status': 'draft',
        'effective_body': originalBody,
        'original_entry_server_id': null,
        'effective_entry_server_id': null,
        'updated_at': now,
      });

      await txn.insert('local_entries', {
        'local_id': _uuid.v4(),
        'experiment_local_id': experimentLocalId,
        'server_id': null,
        'entry_type': 'original',
        'supersedes_server_id': null,
        'body': originalBody,
        'created_at': now,
      });

      await txn.insert('outbox', {
        'mutation_type': 'create_experiment',
        'payload_json': jsonEncode({
          'experimentLocalId': experimentLocalId,
          'title': title,
          'originalBody': originalBody,
        }),
        'status': 'pending',
        'attempts': 0,
        'created_at': now,
      });
    });

    return experimentLocalId;
  }

  Future<void> queueAddendum({
    required String experimentLocalId,
    required String body,
  }) async {
    final db = _database;
    final now = DateTime.now().millisecondsSinceEpoch;
    final expRow = await db.query(
      'local_experiments',
      columns: ['effective_entry_server_id'],
      where: 'local_id = ?',
      whereArgs: [experimentLocalId],
      limit: 1,
    );
    if (expRow.isEmpty) {
      throw StateError('experiment not found');
    }
    final baseEntryId = expRow.first['effective_entry_server_id'] as String?;

    await db.transaction((txn) async {
      await txn.insert('local_entries', {
        'local_id': _uuid.v4(),
        'experiment_local_id': experimentLocalId,
        'server_id': null,
        'entry_type': 'addendum',
        'supersedes_server_id': baseEntryId,
        'body': body,
        'created_at': now,
      });

      await txn.update(
        'local_experiments',
        {
          'effective_body': body,
          'updated_at': now,
        },
        where: 'local_id = ?',
        whereArgs: [experimentLocalId],
      );

      await txn.insert('outbox', {
        'mutation_type': 'add_addendum',
        'payload_json': jsonEncode({
          'experimentLocalId': experimentLocalId,
          'body': body,
          'baseEntryId': baseEntryId,
        }),
        'status': 'pending',
        'attempts': 0,
        'created_at': now,
      });
    });
  }

  Future<void> queueComment({
    required String experimentLocalId,
    required String body,
  }) async {
    final db = _database;
    final now = DateTime.now().millisecondsSinceEpoch;

    await db.transaction((txn) async {
      await txn.insert('local_comments', {
        'local_id': _uuid.v4(),
        'experiment_local_id': experimentLocalId,
        'server_id': null,
        'body': body,
        'created_at': now,
      });

      await txn.insert('outbox', {
        'mutation_type': 'add_comment',
        'payload_json': jsonEncode({
          'experimentLocalId': experimentLocalId,
          'body': body,
        }),
        'status': 'pending',
        'attempts': 0,
        'created_at': now,
      });
    });
  }

  Future<void> queueProposal({
    required String sourceExperimentLocalId,
    required String title,
    required String body,
  }) async {
    final db = _database;
    final now = DateTime.now().millisecondsSinceEpoch;

    await db.transaction((txn) async {
      await txn.insert('local_proposals', {
        'local_id': _uuid.v4(),
        'source_experiment_local_id': sourceExperimentLocalId,
        'server_id': null,
        'title': title,
        'body': body,
        'created_at': now,
      });

      await txn.insert('outbox', {
        'mutation_type': 'create_proposal',
        'payload_json': jsonEncode({
          'sourceExperimentLocalId': sourceExperimentLocalId,
          'title': title,
          'body': body,
        }),
        'status': 'pending',
        'attempts': 0,
        'created_at': now,
      });
    });
  }

  Future<List<ExperimentRecord>> listExperiments() async {
    final rows = await _database.query(
      'local_experiments',
      orderBy: 'updated_at DESC',
    );
    return rows.map(_experimentFromRow).toList(growable: false);
  }

  Future<List<EntryRecord>> listEntries(String experimentLocalId) async {
    final rows = await _database.query(
      'local_entries',
      where: 'experiment_local_id = ?',
      whereArgs: [experimentLocalId],
      orderBy: 'created_at ASC',
    );
    return rows.map(_entryFromRow).toList(growable: false);
  }

  Future<List<CommentRecord>> listComments(String experimentLocalId) async {
    final rows = await _database.query(
      'local_comments',
      where: 'experiment_local_id = ?',
      whereArgs: [experimentLocalId],
      orderBy: 'created_at ASC',
    );
    return rows.map(_commentFromRow).toList(growable: false);
  }

  Future<List<ProposalRecord>> listProposals(String sourceExperimentLocalId) async {
    final rows = await _database.query(
      'local_proposals',
      where: 'source_experiment_local_id = ?',
      whereArgs: [sourceExperimentLocalId],
      orderBy: 'created_at ASC',
    );
    return rows.map(_proposalFromRow).toList(growable: false);
  }

  Future<List<ConflictArtifact>> listConflicts() async {
    final rows = await _database.query(
      'local_conflicts',
      orderBy: 'created_at DESC',
    );
    return rows.map((row) {
      return ConflictArtifact(
        conflictArtifactId: row['server_conflict_artifact_id'] as String,
        experimentId: row['experiment_server_id'] as String,
        clientBaseEntryId: row['client_base_entry_id'] as String?,
        serverLatestEntryId: row['server_latest_entry_id'] as String?,
        createdAt: DateTime.fromMillisecondsSinceEpoch(row['created_at'] as int),
      );
    }).toList(growable: false);
  }

  Future<List<OutboxItem>> listPendingOutbox({int limit = 100}) async {
    final rows = await _database.query(
      'outbox',
      where: 'status IN (?, ?)',
      whereArgs: ['pending', 'error'],
      orderBy: 'id ASC',
      limit: limit,
    );

    return rows.map((row) {
      return OutboxItem(
        id: row['id'] as int,
        mutationType: row['mutation_type'] as String,
        payloadJson: row['payload_json'] as String,
        status: row['status'] as String,
        attempts: row['attempts'] as int,
      );
    }).toList(growable: false);
  }

  Future<void> markOutboxDone(int id) async {
    await _database.update(
      'outbox',
      {'status': 'done', 'last_error': null},
      where: 'id = ?',
      whereArgs: [id],
    );
  }

  Future<void> markOutboxConflict(int id, String error) async {
    await _database.update(
      'outbox',
      {
        'status': 'conflict',
        'last_error': error,
        'attempts': 999,
      },
      where: 'id = ?',
      whereArgs: [id],
    );
  }

  Future<void> markOutboxError(int id, String error) async {
    await _database.rawUpdate(
      'UPDATE outbox SET status = ?, last_error = ?, attempts = attempts + 1 WHERE id = ?',
      ['error', error, id],
    );
  }

  Future<int> getCursor() async {
    final rows = await _database.query(
      'sync_state',
      columns: ['cursor'],
      where: 'id = 1',
      limit: 1,
    );
    if (rows.isEmpty) {
      return 0;
    }
    return (rows.first['cursor'] as int?) ?? 0;
  }

  Future<void> setCursor(int cursor) async {
    await _database.update(
      'sync_state',
      {'cursor': cursor},
      where: 'id = 1',
    );
  }

  Future<String?> getServerExperimentId(String localExperimentId) async {
    final rows = await _database.query(
      'local_experiments',
      columns: ['server_id'],
      where: 'local_id = ?',
      whereArgs: [localExperimentId],
      limit: 1,
    );
    if (rows.isEmpty) {
      return null;
    }
    return rows.first['server_id'] as String?;
  }

  Future<void> attachServerExperimentIdentity({
    required String localExperimentId,
    required String serverExperimentId,
    required String originalEntryServerId,
    required String effectiveEntryServerId,
    required String effectiveBody,
    required String status,
  }) async {
    await _database.update(
      'local_experiments',
      {
        'server_id': serverExperimentId,
        'original_entry_server_id': originalEntryServerId,
        'effective_entry_server_id': effectiveEntryServerId,
        'effective_body': effectiveBody,
        'status': status,
        'updated_at': DateTime.now().millisecondsSinceEpoch,
      },
      where: 'local_id = ?',
      whereArgs: [localExperimentId],
    );
  }

  Future<String> upsertExperimentFromServer(Map<String, dynamic> effectiveJson) async {
    final serverId = effectiveJson['experimentId'] as String;
    final title = effectiveJson['title'] as String? ?? 'Untitled';
    final status = effectiveJson['status'] as String? ?? 'draft';
    final effectiveBody = effectiveJson['effectiveBody'] as String? ?? '';
    final originalEntryId = effectiveJson['originalEntryId'] as String?;
    final effectiveEntryId = effectiveJson['effectiveEntryId'] as String?;

    final existing = await _database.query(
      'local_experiments',
      columns: ['local_id'],
      where: 'server_id = ?',
      whereArgs: [serverId],
      limit: 1,
    );

    final localId = existing.isNotEmpty ? existing.first['local_id'] as String : serverId;

    await _database.insert(
      'local_experiments',
      {
        'local_id': localId,
        'server_id': serverId,
        'title': title,
        'status': status,
        'effective_body': effectiveBody,
        'original_entry_server_id': originalEntryId,
        'effective_entry_server_id': effectiveEntryId,
        'updated_at': DateTime.now().millisecondsSinceEpoch,
      },
      conflictAlgorithm: ConflictAlgorithm.replace,
    );

    return localId;
  }

  Future<void> replaceEntriesForExperiment({
    required String experimentLocalId,
    required List<Map<String, dynamic>> entries,
  }) async {
    final db = _database;
    await db.transaction((txn) async {
      await txn.delete(
        'local_entries',
        where: 'experiment_local_id = ?',
        whereArgs: [experimentLocalId],
      );

      for (final entry in entries) {
        await txn.insert('local_entries', {
          'local_id': _uuid.v4(),
          'experiment_local_id': experimentLocalId,
          'server_id': entry['entryId'] as String,
          'entry_type': entry['entryType'] as String,
          'supersedes_server_id': entry['supersedesEntryId'] as String?,
          'body': entry['body'] as String? ?? '',
          'created_at': DateTime.parse(entry['createdAt'] as String).millisecondsSinceEpoch,
        });
      }
    });
  }

  Future<void> replaceCommentsForExperiment({
    required String experimentLocalId,
    required List<Map<String, dynamic>> comments,
  }) async {
    final db = _database;
    await db.transaction((txn) async {
      await txn.delete(
        'local_comments',
        where: 'experiment_local_id = ?',
        whereArgs: [experimentLocalId],
      );

      for (final comment in comments) {
        await txn.insert('local_comments', {
          'local_id': _uuid.v4(),
          'experiment_local_id': experimentLocalId,
          'server_id': comment['commentId'] as String,
          'body': comment['body'] as String? ?? '',
          'created_at': DateTime.parse(comment['createdAt'] as String).millisecondsSinceEpoch,
        });
      }
    });
  }

  Future<void> replaceProposalsForExperiment({
    required String experimentLocalId,
    required List<Map<String, dynamic>> proposals,
  }) async {
    final db = _database;
    await db.transaction((txn) async {
      await txn.delete(
        'local_proposals',
        where: 'source_experiment_local_id = ?',
        whereArgs: [experimentLocalId],
      );

      for (final proposal in proposals) {
        await txn.insert('local_proposals', {
          'local_id': _uuid.v4(),
          'source_experiment_local_id': experimentLocalId,
          'server_id': proposal['proposalId'] as String,
          'title': proposal['title'] as String? ?? '',
          'body': proposal['body'] as String? ?? '',
          'created_at': DateTime.parse(proposal['createdAt'] as String).millisecondsSinceEpoch,
        });
      }
    });
  }

  Future<void> insertConflictArtifact(Map<String, dynamic> json) async {
    final conflictId = json['conflictArtifactId'] as String;
    final exists = await _database.query(
      'local_conflicts',
      columns: ['local_id'],
      where: 'server_conflict_artifact_id = ?',
      whereArgs: [conflictId],
      limit: 1,
    );
    if (exists.isNotEmpty) {
      return;
    }

    await _database.insert('local_conflicts', {
      'local_id': _uuid.v4(),
      'server_conflict_artifact_id': conflictId,
      'experiment_server_id': json['experimentId'] as String? ?? '',
      'client_base_entry_id': json['clientBaseEntryId'] as String?,
      'server_latest_entry_id': json['serverLatestEntryId'] as String?,
      'payload_json': jsonEncode(json),
      'created_at': DateTime.now().millisecondsSinceEpoch,
    });
  }

  ExperimentRecord _experimentFromRow(Map<String, Object?> row) {
    return ExperimentRecord(
      localId: row['local_id'] as String,
      serverId: row['server_id'] as String?,
      title: row['title'] as String,
      status: row['status'] as String,
      effectiveBody: row['effective_body'] as String,
      originalEntryServerId: row['original_entry_server_id'] as String?,
      effectiveEntryServerId: row['effective_entry_server_id'] as String?,
      updatedAt: DateTime.fromMillisecondsSinceEpoch(row['updated_at'] as int),
    );
  }

  EntryRecord _entryFromRow(Map<String, Object?> row) {
    return EntryRecord(
      localId: row['local_id'] as String,
      experimentLocalId: row['experiment_local_id'] as String,
      serverId: row['server_id'] as String?,
      entryType: row['entry_type'] as String,
      supersedesServerId: row['supersedes_server_id'] as String?,
      body: row['body'] as String,
      createdAt: DateTime.fromMillisecondsSinceEpoch(row['created_at'] as int),
    );
  }

  CommentRecord _commentFromRow(Map<String, Object?> row) {
    return CommentRecord(
      localId: row['local_id'] as String,
      experimentLocalId: row['experiment_local_id'] as String,
      serverId: row['server_id'] as String?,
      body: row['body'] as String,
      createdAt: DateTime.fromMillisecondsSinceEpoch(row['created_at'] as int),
    );
  }

  ProposalRecord _proposalFromRow(Map<String, Object?> row) {
    return ProposalRecord(
      localId: row['local_id'] as String,
      sourceExperimentLocalId: row['source_experiment_local_id'] as String,
      serverId: row['server_id'] as String?,
      title: row['title'] as String,
      body: row['body'] as String,
      createdAt: DateTime.fromMillisecondsSinceEpoch(row['created_at'] as int),
    );
  }

  // ---------------------------------------------------------------------------
  // Enhanced feature tables helper
  // ---------------------------------------------------------------------------

  static Future<void> _createEnhancedTables(Database db) async {
    await db.execute('''
      CREATE TABLE local_protocols (
        protocol_id TEXT PRIMARY KEY,
        creator_user_id TEXT NOT NULL,
        title TEXT NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        status TEXT NOT NULL DEFAULT 'draft',
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_protocol_versions (
        protocol_id TEXT NOT NULL,
        version_num INTEGER NOT NULL,
        author_user_id TEXT NOT NULL,
        body TEXT NOT NULL,
        change_log TEXT NOT NULL DEFAULT '',
        published_at INTEGER NOT NULL,
        PRIMARY KEY (protocol_id, version_num)
      )
    ''');
    await db.execute('''
      CREATE TABLE local_signatures (
        signature_id TEXT PRIMARY KEY,
        experiment_server_id TEXT NOT NULL,
        signer_user_id TEXT NOT NULL,
        signer_email TEXT NOT NULL DEFAULT '',
        role TEXT NOT NULL DEFAULT 'author',
        meaning TEXT NOT NULL DEFAULT '',
        content_hash TEXT NOT NULL DEFAULT '',
        signed_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_notifications (
        notification_id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        event_type TEXT NOT NULL,
        title TEXT NOT NULL,
        body TEXT NOT NULL DEFAULT '',
        experiment_id TEXT,
        is_read INTEGER NOT NULL DEFAULT 0,
        created_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_tags (
        tag_id TEXT NOT NULL,
        experiment_server_id TEXT NOT NULL,
        name TEXT NOT NULL,
        PRIMARY KEY (tag_id, experiment_server_id)
      )
    ''');
    await db.execute('''
      CREATE TABLE local_templates (
        template_id TEXT PRIMARY KEY,
        title TEXT NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        body_template TEXT NOT NULL DEFAULT '',
        sections_json TEXT NOT NULL DEFAULT '[]',
        protocol_id TEXT,
        tags_json TEXT NOT NULL DEFAULT '[]',
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_data_extracts (
        data_extract_id TEXT PRIMARY KEY,
        attachment_id TEXT NOT NULL,
        experiment_server_id TEXT NOT NULL,
        column_headers_json TEXT NOT NULL DEFAULT '[]',
        row_count INTEGER NOT NULL DEFAULT 0,
        sample_rows_json TEXT NOT NULL DEFAULT '[]',
        parsed_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_chart_configs (
        chart_config_id TEXT PRIMARY KEY,
        experiment_server_id TEXT NOT NULL,
        data_extract_id TEXT NOT NULL,
        chart_type TEXT NOT NULL DEFAULT 'line',
        title TEXT NOT NULL DEFAULT '',
        x_column TEXT NOT NULL DEFAULT '',
        y_columns_json TEXT NOT NULL DEFAULT '[]',
        options_json TEXT NOT NULL DEFAULT '{}',
        created_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_deviations (
        deviation_id TEXT PRIMARY KEY,
        experiment_server_id TEXT NOT NULL,
        protocol_id TEXT NOT NULL,
        reported_by TEXT NOT NULL,
        description TEXT NOT NULL,
        severity TEXT NOT NULL DEFAULT 'minor',
        created_at INTEGER NOT NULL
      )
    ''');
    await db.execute('''
      CREATE TABLE local_previews (
        preview_id TEXT PRIMARY KEY,
        attachment_id TEXT NOT NULL UNIQUE,
        preview_type TEXT NOT NULL DEFAULT 'thumbnail',
        mime_type TEXT NOT NULL DEFAULT 'image/png',
        width INTEGER NOT NULL DEFAULT 0,
        height INTEGER NOT NULL DEFAULT 0,
        data_base64 TEXT NOT NULL DEFAULT '',
        created_at INTEGER NOT NULL
      )
    ''');
  }

  // ---------------------------------------------------------------------------
  // Protocol local storage
  // ---------------------------------------------------------------------------

  Future<void> upsertProtocol(Map<String, dynamic> json) async {
    final now = DateTime.now().millisecondsSinceEpoch;
    await _database.insert(
      'local_protocols',
      {
        'protocol_id': json['protocolId'] as String,
        'creator_user_id': json['creatorUserId'] as String? ?? '',
        'title': json['title'] as String? ?? '',
        'description': json['description'] as String? ?? '',
        'status': json['status'] as String? ?? 'draft',
        'created_at': json['createdAt'] != null
            ? DateTime.parse(json['createdAt'] as String).millisecondsSinceEpoch
            : now,
        'updated_at': now,
      },
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
  }

  Future<List<Map<String, Object?>>> listLocalProtocols() async {
    return _database.query('local_protocols', orderBy: 'updated_at DESC');
  }

  // ---------------------------------------------------------------------------
  // Notification local storage
  // ---------------------------------------------------------------------------

  Future<void> replaceNotifications(List<Map<String, dynamic>> notifications) async {
    await _database.transaction((txn) async {
      await txn.delete('local_notifications');
      for (final n in notifications) {
        await txn.insert('local_notifications', {
          'notification_id': n['notificationId'] as String,
          'user_id': n['userId'] as String? ?? '',
          'event_type': n['eventType'] as String? ?? '',
          'title': n['title'] as String? ?? '',
          'body': n['body'] as String? ?? '',
          'experiment_id': n['experimentId'] as String?,
          'is_read': (n['isRead'] as bool? ?? false) ? 1 : 0,
          'created_at': n['createdAt'] != null
              ? DateTime.parse(n['createdAt'] as String).millisecondsSinceEpoch
              : DateTime.now().millisecondsSinceEpoch,
        });
      }
    });
  }

  Future<List<Map<String, Object?>>> listLocalNotifications({bool unreadOnly = false}) async {
    if (unreadOnly) {
      return _database.query('local_notifications',
          where: 'is_read = 0', orderBy: 'created_at DESC');
    }
    return _database.query('local_notifications', orderBy: 'created_at DESC');
  }

  Future<int> countUnreadNotifications() async {
    final result = await _database.rawQuery(
        'SELECT COUNT(*) as cnt FROM local_notifications WHERE is_read = 0');
    return (result.first['cnt'] as int?) ?? 0;
  }

  Future<void> markNotificationReadLocal(String notificationId) async {
    await _database.update(
      'local_notifications',
      {'is_read': 1},
      where: 'notification_id = ?',
      whereArgs: [notificationId],
    );
  }

  // ---------------------------------------------------------------------------
  // Signature local storage
  // ---------------------------------------------------------------------------

  Future<void> replaceSignaturesForExperiment({
    required String experimentServerId,
    required List<Map<String, dynamic>> signatures,
  }) async {
    await _database.transaction((txn) async {
      await txn.delete('local_signatures',
          where: 'experiment_server_id = ?', whereArgs: [experimentServerId]);
      for (final s in signatures) {
        await txn.insert('local_signatures', {
          'signature_id': s['signatureId'] as String,
          'experiment_server_id': experimentServerId,
          'signer_user_id': s['signerUserId'] as String? ?? '',
          'signer_email': s['signerEmail'] as String? ?? '',
          'role': s['signatureType'] as String? ?? s['role'] as String? ?? 'author',
          'meaning': s['signatureType'] as String? ?? s['meaning'] as String? ?? '',
          'content_hash': s['contentHash'] as String? ?? '',
          'signed_at': s['signedAt'] != null
              ? DateTime.parse(s['signedAt'] as String).millisecondsSinceEpoch
              : DateTime.now().millisecondsSinceEpoch,
        });
      }
    });
  }

  Future<List<Map<String, Object?>>> listLocalSignatures(String experimentServerId) async {
    return _database.query('local_signatures',
        where: 'experiment_server_id = ?',
        whereArgs: [experimentServerId],
        orderBy: 'signed_at ASC');
  }

  // ---------------------------------------------------------------------------
  // Tag local storage
  // ---------------------------------------------------------------------------

  Future<void> replaceTagsForExperiment({
    required String experimentServerId,
    required List<Map<String, dynamic>> tags,
  }) async {
    await _database.transaction((txn) async {
      await txn.delete('local_tags',
          where: 'experiment_server_id = ?', whereArgs: [experimentServerId]);
      for (final t in tags) {
        await txn.insert('local_tags', {
          'tag_id': t['tagId'] as String,
          'experiment_server_id': experimentServerId,
          'name': t['name'] as String,
        });
      }
    });
  }

  Future<List<Map<String, Object?>>> listLocalTags(String experimentServerId) async {
    return _database.query('local_tags',
        where: 'experiment_server_id = ?',
        whereArgs: [experimentServerId],
        orderBy: 'name ASC');
  }

  // ---------------------------------------------------------------------------
  // Template local storage
  // ---------------------------------------------------------------------------

  Future<void> replaceTemplates(List<Map<String, dynamic>> templates) async {
    await _database.transaction((txn) async {
      await txn.delete('local_templates');
      for (final t in templates) {
        await txn.insert('local_templates', {
          'template_id': t['templateId'] as String,
          'title': t['title'] as String? ?? '',
          'description': t['description'] as String? ?? '',
          'body_template': t['bodyTemplate'] as String? ?? '',
          'sections_json': jsonEncode(t['sections'] ?? []),
          'protocol_id': t['protocolId'] as String?,
          'tags_json': jsonEncode(t['tags'] ?? []),
          'created_at': t['createdAt'] != null
              ? DateTime.parse(t['createdAt'] as String).millisecondsSinceEpoch
              : DateTime.now().millisecondsSinceEpoch,
          'updated_at': t['updatedAt'] != null
              ? DateTime.parse(t['updatedAt'] as String).millisecondsSinceEpoch
              : DateTime.now().millisecondsSinceEpoch,
        });
      }
    });
  }

  Future<List<Map<String, Object?>>> listLocalTemplates() async {
    return _database.query('local_templates', orderBy: 'updated_at DESC');
  }

  // ---------------------------------------------------------------------------
  // Preview local cache
  // ---------------------------------------------------------------------------

  Future<void> cachePreview(Map<String, dynamic> json) async {
    await _database.insert(
      'local_previews',
      {
        'preview_id': json['previewId'] as String,
        'attachment_id': json['attachmentId'] as String,
        'preview_type': json['previewType'] as String? ?? 'thumbnail',
        'mime_type': json['mimeType'] as String? ?? 'image/png',
        'width': (json['width'] as num?)?.toInt() ?? 0,
        'height': (json['height'] as num?)?.toInt() ?? 0,
        'data_base64': json['dataBase64'] as String? ?? '',
        'created_at': DateTime.now().millisecondsSinceEpoch,
      },
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
  }

  Future<Map<String, Object?>?> getLocalPreview(String attachmentId) async {
    final rows = await _database.query('local_previews',
        where: 'attachment_id = ?', whereArgs: [attachmentId], limit: 1);
    return rows.isEmpty ? null : rows.first;
  }
}
