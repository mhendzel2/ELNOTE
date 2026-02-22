import 'dart:convert';

import 'package:csv/csv.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../data/api_client.dart';
import '../data/sync_service.dart';

class DataImportScreen extends StatefulWidget {
  const DataImportScreen({super.key, required this.sync});

  final SyncService sync;

  @override
  State<DataImportScreen> createState() => _DataImportScreenState();
}

class _DataImportScreenState extends State<DataImportScreen> {
  final Map<String, _ImportResult> _results = <String, _ImportResult>{};
  final Set<String> _running = <String>{};
  bool _runningAccessImport = false;
  Map<String, dynamic>? _lastAccessImportResult;

  ApiClient get _api => widget.sync.api;

  static const List<_ImportSpec> _specs = <_ImportSpec>[
    _ImportSpec(
      type: 'storage',
      label: 'Storage',
      requiredKeys: <String>['name'],
      optionalKeys: <String>['locationType', 'description'],
      aliases: <String, String>{
        'type': 'locationType',
        'location_type': 'locationType',
      },
    ),
    _ImportSpec(
      type: 'boxes',
      label: 'Boxes',
      requiredKeys: <String>['boxNo'],
      optionalKeys: <String>[
        'boxType',
        'owner',
        'label',
        'location',
        'drawer',
        'position',
        'storageId',
      ],
      aliases: <String, String>{
        'box_number': 'boxNo',
        'storage_id': 'storageId',
      },
    ),
    _ImportSpec(
      type: 'antibodies',
      label: 'Antibodies',
      requiredKeys: <String>['antibodyName'],
      optionalKeys: <String>[
        'catalogNo',
        'company',
        'lotNumber',
        'expiryDate',
        'class',
        'antigen',
        'host',
        'investigator',
        'expId',
        'notes',
        'boxId',
        'location',
        'quantity',
        'isDepleted',
      ],
      aliases: <String, String>{
        'catalog_number': 'catalogNo',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
        'class_name': 'class',
      },
    ),
    _ImportSpec(
      type: 'cell-lines',
      label: 'Cell Lines',
      requiredKeys: <String>['cellLineName'],
      optionalKeys: <String>[
        'lotNumber',
        'expiryDate',
        'selection',
        'species',
        'parentalCell',
        'medium',
        'obtainFrom',
        'cellType',
        'boxId',
        'location',
        'owner',
        'label',
        'notes',
        'isDepleted',
      ],
      aliases: <String, String>{
        'cell_line_name': 'cellLineName',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
        'obtain_from': 'obtainFrom',
        'cell_type': 'cellType',
      },
    ),
    _ImportSpec(
      type: 'viruses',
      label: 'Viruses',
      requiredKeys: <String>['virusName'],
      optionalKeys: <String>[
        'virusType',
        'lotNumber',
        'expiryDate',
        'boxId',
        'location',
        'owner',
        'label',
        'notes',
        'isDepleted',
      ],
      aliases: <String, String>{
        'virus_type': 'virusType',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
      },
    ),
    _ImportSpec(
      type: 'dna',
      label: 'DNA',
      requiredKeys: <String>['dnaName'],
      optionalKeys: <String>[
        'dnaType',
        'lotNumber',
        'expiryDate',
        'boxId',
        'location',
        'owner',
        'label',
        'notes',
        'isDepleted',
      ],
      aliases: <String, String>{
        'dna_type': 'dnaType',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
      },
    ),
    _ImportSpec(
      type: 'oligos',
      label: 'Oligos',
      requiredKeys: <String>['oligoName'],
      optionalKeys: <String>[
        'sequence',
        'oligoType',
        'lotNumber',
        'expiryDate',
        'boxId',
        'location',
        'owner',
        'label',
        'notes',
        'isDepleted',
      ],
      aliases: <String, String>{
        'oligo_type': 'oligoType',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
      },
    ),
    _ImportSpec(
      type: 'chemicals',
      label: 'Chemicals',
      requiredKeys: <String>['chemicalName'],
      optionalKeys: <String>[
        'catalogNo',
        'company',
        'chemType',
        'lotNumber',
        'expiryDate',
        'boxId',
        'location',
        'owner',
        'label',
        'notes',
        'isDepleted',
      ],
      aliases: <String, String>{
        'catalog_number': 'catalogNo',
        'chem_type': 'chemType',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
      },
    ),
    _ImportSpec(
      type: 'molecular',
      label: 'Molecular',
      requiredKeys: <String>['mrName'],
      optionalKeys: <String>[
        'mrType',
        'lotNumber',
        'expiryDate',
        'boxId',
        'location',
        'position',
        'owner',
        'label',
        'notes',
        'isDepleted',
      ],
      aliases: <String, String>{
        'mr_type': 'mrType',
        'expiry': 'expiryDate',
        'exp_date': 'expiryDate',
        'expiration_date': 'expiryDate',
      },
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
          child: Align(
            alignment: Alignment.centerLeft,
            child: Text(
              'Data Import',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
          ),
        ),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
            children: [
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text(
                        'One-step legacy import (.mdb/.accdb)',
                        style: TextStyle(fontWeight: FontWeight.w600),
                      ),
                      const SizedBox(height: 8),
                      const Text(
                        'Upload an Access database and import all detected reagent tables in dependency order.',
                      ),
                      const SizedBox(height: 10),
                      Wrap(
                        spacing: 8,
                        runSpacing: 8,
                        children: [
                          FilledButton.icon(
                            onPressed: _runningAccessImport
                                ? null
                                : _importAccessDatabase,
                            icon: Icon(Icons.folder_zip),
                            label: Text('Import .mdb/.accdb'),
                          ),
                          if (_lastAccessImportResult != null)
                            OutlinedButton.icon(
                              onPressed: _showAccessImportSummary,
                              icon: Icon(Icons.summarize),
                              label: Text('View Last Report'),
                            ),
                        ],
                      ),
                      if (_runningAccessImport) ...[
                        const SizedBox(height: 12),
                        const LinearProgressIndicator(),
                      ],
                      if (_lastAccessImportResult != null) ...[
                        const SizedBox(height: 12),
                        Text(
                          'Last import: ${_lastAccessImportResult!['fileName'] ?? 'unknown'}'
                          ' | imported ${(_lastAccessImportResult!['totalImported'] as num?)?.toInt() ?? 0}',
                        ),
                      ],
                      const Divider(height: 24),
                      const Text(
                        'Import existing reagent data via CSV or JSON using the bulk import API.',
                      ),
                      const SizedBox(height: 8),
                      const Text(
                        'Recommended order: storage -> boxes -> antibodies -> cell-lines -> viruses -> dna -> oligos -> chemicals -> molecular',
                      ),
                      const SizedBox(height: 8),
                      const Text(
                        'Tip: date fields should use YYYY-MM-DD; leave blank for no date.',
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 12),
              ..._specs.map(_buildImportCard),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildImportCard(_ImportSpec spec) {
    final running = _running.contains(spec.type);
    final result = _results[spec.type];
    final headers = spec.allKeys.join(',');

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    spec.label,
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                ),
                if (running)
                  const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
              ],
            ),
            const SizedBox(height: 6),
            SelectableText(
              'POST /v1/reagents/${spec.type}/import',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            const SizedBox(height: 8),
            Text(
              'Required: ${spec.requiredKeys.join(', ')}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            Text(
              'Supported: $headers',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                FilledButton.icon(
                  onPressed: running ? null : () => _importCsv(spec),
                  icon: const Icon(Icons.upload_file),
                  label: const Text('Import CSV'),
                ),
                OutlinedButton.icon(
                  onPressed: running ? null : () => _importJson(spec),
                  icon: const Icon(Icons.data_object),
                  label: const Text('Import JSON'),
                ),
                TextButton.icon(
                  onPressed: () => _copyTemplateHeader(headers),
                  icon: const Icon(Icons.content_copy),
                  label: const Text('Copy Headers'),
                ),
              ],
            ),
            if (result != null) ...[
              const SizedBox(height: 12),
              Text(
                '${result.fileName} (${result.source}) - imported ${result.imported}'
                '${result.errors.isNotEmpty ? ', errors ${result.errors.length}' : ''}'
                ' at ${_formatTime(result.finishedAt)}',
              ),
              if (result.errors.isNotEmpty)
                Align(
                  alignment: Alignment.centerLeft,
                  child: TextButton(
                    onPressed: () => _showErrors(
                        '${spec.label} Import Errors', result.errors),
                    child: const Text('View errors'),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }

  String _formatTime(DateTime dt) {
    final local = dt.toLocal();
    final hh = local.hour.toString().padLeft(2, '0');
    final mm = local.minute.toString().padLeft(2, '0');
    final ss = local.second.toString().padLeft(2, '0');
    return '$hh:$mm:$ss';
  }

  Future<void> _importCsv(_ImportSpec spec) async {
    final selected = await _pickFile(
      allowedExtensions: const ['csv'],
      dialogTitle: '${spec.label} CSV Import',
    );
    if (selected == null) return;

    await _runImport(
      spec: spec,
      source: 'CSV',
      fileName: selected.name,
      loader: () async {
        final text = utf8.decode(selected.bytes, allowMalformed: true);
        return _parseCsv(spec, text);
      },
    );
  }

  Future<void> _importJson(_ImportSpec spec) async {
    final selected = await _pickFile(
      allowedExtensions: const ['json'],
      dialogTitle: '${spec.label} JSON Import',
    );
    if (selected == null) return;

    await _runImport(
      spec: spec,
      source: 'JSON',
      fileName: selected.name,
      loader: () async {
        final text = utf8.decode(selected.bytes, allowMalformed: true);
        return _parseJson(spec, text);
      },
    );
  }

  Future<void> _importAccessDatabase() async {
    if (_runningAccessImport) {
      return;
    }

    final selected = await _pickFile(
      allowedExtensions: const ['mdb', 'accdb'],
      dialogTitle: 'Access Database Import',
    );
    if (selected == null) return;

    setState(() {
      _runningAccessImport = true;
    });
    try {
      final response = await _api.importAccessDatabase(
        fileBytes: selected.bytes,
        fileName: selected.name,
      );
      if (!mounted) return;
      setState(() {
        _lastAccessImportResult = response;
      });
      final totalImported = (response['totalImported'] as num?)?.toInt() ?? 0;
      _showSnack('Access import completed: $totalImported rows imported');
      _showAccessImportSummary();
    } on ApiException catch (e) {
      _showSnack('Access import failed: ${e.message}');
    } catch (e) {
      _showSnack('Access import failed: $e');
    } finally {
      if (mounted) {
        setState(() {
          _runningAccessImport = false;
        });
      }
    }
  }

  Future<void> _runImport({
    required _ImportSpec spec,
    required String source,
    required String fileName,
    required Future<_PreparedImport> Function() loader,
  }) async {
    if (_running.contains(spec.type)) {
      return;
    }

    setState(() {
      _running.add(spec.type);
    });

    try {
      final prepared = await loader();
      if (prepared.items.isEmpty && prepared.errors.isEmpty) {
        _showSnack('No rows found in $fileName');
        return;
      }

      int imported = 0;
      final errors = List<String>.from(prepared.errors);

      if (prepared.items.isNotEmpty) {
        final response =
            await _api.bulkImportReagents(spec.type, prepared.items);
        imported = (response['imported'] as num?)?.toInt() ?? 0;
        final serverErrors =
            (response['errors'] as List<dynamic>? ?? <dynamic>[])
                .map((e) => e.toString())
                .toList(growable: false);
        errors.addAll(serverErrors);
      }

      if (!mounted) return;
      setState(() {
        _results[spec.type] = _ImportResult(
          source: source,
          fileName: fileName,
          imported: imported,
          errors: errors,
          finishedAt: DateTime.now(),
        );
      });

      _showSnack(
        '${spec.label}: imported $imported'
        '${errors.isNotEmpty ? ' (${errors.length} errors)' : ''}',
      );

      if (errors.isNotEmpty) {
        _showErrors('${spec.label} Import Errors', errors);
      }
    } on ApiException catch (e) {
      _showSnack('${spec.label} import failed: ${e.message}');
    } catch (e) {
      _showSnack('${spec.label} import failed: $e');
    } finally {
      if (mounted) {
        setState(() {
          _running.remove(spec.type);
        });
      }
    }
  }

  _PreparedImport _parseCsv(_ImportSpec spec, String csvString) {
    final rows = const CsvToListConverter(
      eol: '\n',
      shouldParseNumbers: false,
    ).convert(csvString);

    if (rows.isEmpty) {
      return _PreparedImport(
          items: const [], errors: const ['CSV file is empty']);
    }

    final headerCells = rows.first
        .map((cell) => cell.toString().trim())
        .toList(growable: false);
    final mappedHeaders = _mapHeaders(spec, headerCells);

    final unknownHeaders = <String>[];
    for (int i = 0; i < headerCells.length; i++) {
      if (mappedHeaders[i] == null && headerCells[i].isNotEmpty) {
        unknownHeaders.add(headerCells[i]);
      }
    }

    final errors = <String>[];
    if (unknownHeaders.isNotEmpty) {
      errors.add('ignored headers: ${unknownHeaders.join(', ')}');
    }

    final items = <Map<String, dynamic>>[];
    for (int rowIndex = 1; rowIndex < rows.length; rowIndex++) {
      final row = rows[rowIndex];
      final item = <String, dynamic>{};

      for (int col = 0; col < headerCells.length && col < row.length; col++) {
        final key = mappedHeaders[col];
        if (key == null) continue;
        item[key] = _coerceValue(key, row[col]);
      }

      if (_isCompletelyEmpty(item)) {
        continue;
      }

      final missing = _missingRequired(spec.requiredKeys, item);
      if (missing.isNotEmpty) {
        errors.add('row ${rowIndex + 1}: missing ${missing.join(', ')}');
        continue;
      }

      items.add(item);
    }

    return _PreparedImport(items: items, errors: errors);
  }

  _PreparedImport _parseJson(_ImportSpec spec, String jsonText) {
    dynamic decoded;
    try {
      decoded = jsonDecode(jsonText);
    } catch (_) {
      return _PreparedImport(items: const [], errors: const ['invalid JSON']);
    }

    List<dynamic> rawItems;
    if (decoded is List<dynamic>) {
      rawItems = decoded;
    } else if (decoded is Map<String, dynamic> &&
        decoded['items'] is List<dynamic>) {
      rawItems = decoded['items'] as List<dynamic>;
    } else {
      return _PreparedImport(
        items: const [],
        errors: const [
          'JSON must be an array or an object with an "items" array'
        ],
      );
    }

    final items = <Map<String, dynamic>>[];
    final errors = <String>[];
    for (int i = 0; i < rawItems.length; i++) {
      final raw = rawItems[i];
      if (raw is! Map) {
        errors.add('item ${i + 1}: must be an object');
        continue;
      }

      final normalized = _normalizeMapKeys(spec, raw);
      if (_isCompletelyEmpty(normalized)) {
        continue;
      }

      final missing = _missingRequired(spec.requiredKeys, normalized);
      if (missing.isNotEmpty) {
        errors.add('item ${i + 1}: missing ${missing.join(', ')}');
        continue;
      }
      items.add(normalized);
    }

    return _PreparedImport(items: items, errors: errors);
  }

  Map<int, String?> _mapHeaders(_ImportSpec spec, List<String> headers) {
    final lookup = <String, String>{};
    for (final key in spec.allKeys) {
      lookup[_normalizeKey(key)] = key;
    }
    spec.aliases.forEach((alias, canonical) {
      lookup[_normalizeKey(alias)] = canonical;
    });

    final mapped = <int, String?>{};
    for (int i = 0; i < headers.length; i++) {
      final normalized = _normalizeKey(headers[i]);
      mapped[i] = lookup[normalized];
    }
    return mapped;
  }

  Map<String, dynamic> _normalizeMapKeys(_ImportSpec spec, Map raw) {
    final lookup = <String, String>{};
    for (final key in spec.allKeys) {
      lookup[_normalizeKey(key)] = key;
    }
    spec.aliases.forEach((alias, canonical) {
      lookup[_normalizeKey(alias)] = canonical;
    });

    final out = <String, dynamic>{};
    for (final entry in raw.entries) {
      final k = entry.key?.toString() ?? '';
      final canonical = lookup[_normalizeKey(k)];
      if (canonical == null) {
        continue;
      }
      out[canonical] = _coerceValue(canonical, entry.value);
    }
    return out;
  }

  String _normalizeKey(String value) {
    return value.replaceAll(RegExp(r'[^A-Za-z0-9]'), '').toLowerCase();
  }

  dynamic _coerceValue(String key, dynamic value) {
    if (value == null) {
      return key == 'boxId' || key == 'storageId' ? null : '';
    }
    if (key == 'boxId' || key == 'storageId') {
      if (value is num) return value.toInt();
      final s = value.toString().trim();
      if (s.isEmpty) return null;
      return int.tryParse(s);
    }
    if (key == 'isDepleted') {
      if (value is bool) return value;
      final s = value.toString().trim().toLowerCase();
      return s == '1' || s == 'true' || s == 'yes' || s == 'y';
    }
    return value.toString().trim();
  }

  List<String> _missingRequired(
      List<String> requiredKeys, Map<String, dynamic> item) {
    final missing = <String>[];
    for (final key in requiredKeys) {
      final value = item[key];
      if (value == null) {
        missing.add(key);
        continue;
      }
      if (value is String && value.trim().isEmpty) {
        missing.add(key);
      }
    }
    return missing;
  }

  bool _isCompletelyEmpty(Map<String, dynamic> item) {
    if (item.isEmpty) return true;
    for (final value in item.values) {
      if (value == null) continue;
      if (value is String && value.trim().isEmpty) continue;
      return false;
    }
    return true;
  }

  Future<_PickedFile?> _pickFile({
    required List<String> allowedExtensions,
    required String dialogTitle,
  }) async {
    final result = await FilePicker.platform.pickFiles(
      dialogTitle: dialogTitle,
      type: FileType.custom,
      allowedExtensions: allowedExtensions,
      allowMultiple: false,
      withData: true,
    );
    if (result == null || result.files.isEmpty) {
      return null;
    }

    final file = result.files.single;
    final bytes = file.bytes;
    if (bytes == null) {
      _showSnack('Unable to read file bytes for ${file.name}');
      return null;
    }
    return _PickedFile(name: file.name, bytes: bytes);
  }

  Future<void> _copyTemplateHeader(String headers) async {
    await Clipboard.setData(ClipboardData(text: headers));
    _showSnack('CSV header copied to clipboard');
  }

  void _showSnack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
  }

  void _showErrors(String title, List<String> errors) {
    if (!mounted) return;
    showDialog<void>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: SizedBox(
          width: 680,
          height: 380,
          child: SingleChildScrollView(
            child: Text(errors.join('\n')),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  void _showAccessImportSummary() {
    final payload = _lastAccessImportResult;
    if (!mounted || payload == null) {
      return;
    }

    final encoder = const JsonEncoder.withIndent('  ');
    final pretty = encoder.convert(payload);
    showDialog<void>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Access Import Report'),
        content: SizedBox(
          width: 760,
          height: 460,
          child: SingleChildScrollView(
            child: SelectableText(pretty),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }
}

class _ImportSpec {
  const _ImportSpec({
    required this.type,
    required this.label,
    required this.requiredKeys,
    required this.optionalKeys,
    this.aliases = const <String, String>{},
  });

  final String type;
  final String label;
  final List<String> requiredKeys;
  final List<String> optionalKeys;
  final Map<String, String> aliases;

  List<String> get allKeys => <String>[...requiredKeys, ...optionalKeys];
}

class _PreparedImport {
  const _PreparedImport({required this.items, required this.errors});

  final List<Map<String, dynamic>> items;
  final List<String> errors;
}

class _ImportResult {
  const _ImportResult({
    required this.source,
    required this.fileName,
    required this.imported,
    required this.errors,
    required this.finishedAt,
  });

  final String source;
  final String fileName;
  final int imported;
  final List<String> errors;
  final DateTime finishedAt;
}

class _PickedFile {
  const _PickedFile({required this.name, required this.bytes});

  final String name;
  final Uint8List bytes;
}
