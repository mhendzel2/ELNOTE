import 'dart:io';

import 'package:csv/csv.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/local_database.dart';
import '../data/sync_service.dart';
import '../models/models.dart';

// ===========================================================================
// ReagentsScreen — tabbed reagent inventory browser/editor
// ===========================================================================

class ReagentsScreen extends StatefulWidget {
  const ReagentsScreen({super.key, required this.db, required this.sync});

  final LocalDatabase db;
  final SyncService sync;

  @override
  State<ReagentsScreen> createState() => _ReagentsScreenState();
}

class _ReagentsScreenState extends State<ReagentsScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabCtl;
  final _searchCtl = TextEditingController();
  bool _showDepleted = false;

  @override
  void initState() {
    super.initState();
    _tabCtl = TabController(length: 9, vsync: this);
  }

  @override
  void dispose() {
    _tabCtl.dispose();
    _searchCtl.dispose();
    super.dispose();
  }

  ApiClient get api => widget.sync.api;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        // Toolbar: search + show-depleted toggle
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _searchCtl,
                  decoration: const InputDecoration(
                    prefixIcon: Icon(Icons.search),
                    hintText: 'Search reagents…',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => setState(() {}),
                ),
              ),
              const SizedBox(width: 12),
              FilterChip(
                label: const Text('Show depleted'),
                selected: _showDepleted,
                onSelected: (v) => setState(() => _showDepleted = v),
              ),
            ],
          ),
        ),
        TabBar(
          controller: _tabCtl,
          isScrollable: true,
          tabs: const [
            Tab(text: 'Storage'),
            Tab(text: 'Boxes'),
            Tab(text: 'Antibodies'),
            Tab(text: 'Cell Lines'),
            Tab(text: 'Viruses'),
            Tab(text: 'DNA'),
            Tab(text: 'Oligos'),
            Tab(text: 'Chemicals'),
            Tab(text: 'Molecular'),
          ],
        ),
        Expanded(
          child: TabBarView(
            controller: _tabCtl,
            children: [
              _StorageTab(api: api),
              _BoxTab(api: api, q: _searchCtl.text),
              _ReagentListTab<ReagentAntibody>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listAntibodies(q: q, depleted: dep),
                nameOf: (a) => a.antibodyName,
                subtitleOf: (a) =>
                    '${a.company} | ${a.catalogNo} | ${a.antigen}',
                onAdd: () => _addAntibody(),
                onTap: (a) => _editAntibody(a),
                onDelete: (a) => api.deleteAntibody(a.id),
                isDepletedOf: (a) => a.isDepleted,
                onImportCsv: () => _importCsv('antibodies', ['antibodyName','catalogNo','company','class','antigen','host','investigator','notes','location','quantity']),
              ),
              _ReagentListTab<ReagentCellLine>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listCellLines(q: q, depleted: dep),
                nameOf: (c) => c.cellLineName,
                subtitleOf: (c) => '${c.species} | ${c.medium}',
                onAdd: () => _addCellLine(),
                onTap: (c) => _editCellLine(c),
                onDelete: (c) => api.deleteCellLine(c.id),
                isDepletedOf: (c) => c.isDepleted,
                onImportCsv: () => _importCsv('cell-lines', ['cellLineName','selection','species','parentalCell','medium','obtainFrom','cellType','location','owner','label','notes']),
              ),
              _ReagentListTab<ReagentVirus>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listViruses(q: q, depleted: dep),
                nameOf: (v) => v.virusName,
                subtitleOf: (v) => '${v.virusType} | Owner: ${v.owner}',
                onAdd: () => _addVirus(),
                onTap: (v) => _editVirus(v),
                onDelete: (v) => api.deleteVirus(v.id),
                isDepletedOf: (v) => v.isDepleted,
                onImportCsv: () => _importCsv('viruses', ['virusName','virusType','location','owner','label','notes']),
              ),
              _ReagentListTab<ReagentDNA>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listDNA(q: q, depleted: dep),
                nameOf: (d) => d.dnaName,
                subtitleOf: (d) => '${d.dnaType} | Owner: ${d.owner}',
                onAdd: () => _addDNA(),
                onTap: (d) => _editDNA(d),
                onDelete: (d) => api.deleteDNA(d.id),
                isDepletedOf: (d) => d.isDepleted,
                onImportCsv: () => _importCsv('dna', ['dnaName','dnaType','location','owner','label','notes']),
              ),
              _ReagentListTab<ReagentOligo>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listOligos(q: q, depleted: dep),
                nameOf: (o) => o.oligoName,
                subtitleOf: (o) =>
                    '${o.oligoType} | ${o.sequence}',
                onAdd: () => _addOligo(),
                onTap: (o) => _editOligo(o),
                onDelete: (o) => api.deleteOligo(o.id),
                isDepletedOf: (o) => o.isDepleted,
                onImportCsv: () => _importCsv('oligos', ['oligoName','sequence','oligoType','location','owner','label','notes']),
              ),
              _ReagentListTab<ReagentChemical>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listChemicals(q: q, depleted: dep),
                nameOf: (c) => c.chemicalName,
                subtitleOf: (c) =>
                    '${c.company} | ${c.catalogNo} | ${c.chemType}',
                onAdd: () => _addChemical(),
                onTap: (c) => _editChemical(c),
                onDelete: (c) => api.deleteChemical(c.id),
                isDepletedOf: (c) => c.isDepleted,
                onImportCsv: () => _importCsv('chemicals', ['chemicalName','catalogNo','company','chemType','location','owner','label','notes']),
              ),
              _ReagentListTab<ReagentMolecular>(
                api: api,
                q: _searchCtl.text,
                depleted: _showDepleted,
                fetch: (q, dep) => api.listMolecular(q: q, depleted: dep),
                nameOf: (m) => m.mrName,
                subtitleOf: (m) =>
                    '${m.mrType} | Owner: ${m.owner}',
                onAdd: () => _addMolecular(),
                onTap: (m) => _editMolecular(m),
                onDelete: (m) => api.deleteMolecular(m.id),
                isDepletedOf: (m) => m.isDepleted,
                onImportCsv: () => _importCsv('molecular', ['mrName','mrType','location','position','owner','label','notes']),
              ),
            ],
          ),
        ),
      ],
    );
  }

  // =========================================================================
  // Add / Edit dialogs
  // =========================================================================

  Future<void> _addAntibody() async {
    final fields = await _showFormDialog('New Antibody', [
      'antibodyName', 'catalogNo', 'company', 'class', 'antigen',
      'host', 'investigator', 'notes', 'location', 'quantity',
    ]);
    if (fields == null) return;
    await api.createAntibody(fields);
    setState(() {});
  }

  Future<void> _editAntibody(ReagentAntibody a) async {
    final fields = await _showFormDialog('Edit Antibody', [
      'antibodyName', 'catalogNo', 'company', 'class', 'antigen',
      'host', 'investigator', 'notes', 'location', 'quantity',
    ], initial: {
      'antibodyName': a.antibodyName,
      'catalogNo': a.catalogNo,
      'company': a.company,
      'class': a.antibodyClass,
      'antigen': a.antigen,
      'host': a.host,
      'investigator': a.investigator,
      'notes': a.notes,
      'location': a.location,
      'quantity': a.quantity,
    });
    if (fields == null) return;
    await api.updateAntibody(a.id, fields);
    setState(() {});
  }

  Future<void> _addCellLine() async {
    final fields = await _showFormDialog('New Cell Line', [
      'cellLineName', 'selection', 'species', 'parentalCell', 'medium',
      'obtainFrom', 'cellType', 'notes', 'location', 'owner', 'label',
    ]);
    if (fields == null) return;
    await api.createCellLine(fields);
    setState(() {});
  }

  Future<void> _editCellLine(ReagentCellLine c) async {
    final fields = await _showFormDialog('Edit Cell Line', [
      'cellLineName', 'selection', 'species', 'parentalCell', 'medium',
      'obtainFrom', 'cellType', 'notes', 'location', 'owner', 'label',
    ], initial: {
      'cellLineName': c.cellLineName,
      'selection': c.selection,
      'species': c.species,
      'parentalCell': c.parentalCell,
      'medium': c.medium,
      'obtainFrom': c.obtainFrom,
      'cellType': c.cellType,
      'notes': c.notes,
      'location': c.location,
      'owner': c.owner,
      'label': c.label,
    });
    if (fields == null) return;
    await api.updateCellLine(c.id, fields);
    setState(() {});
  }

  Future<void> _addVirus() async {
    final fields = await _showFormDialog('New Virus', [
      'virusName', 'virusType', 'notes', 'location', 'owner', 'label',
    ]);
    if (fields == null) return;
    await api.createVirus(fields);
    setState(() {});
  }

  Future<void> _editVirus(ReagentVirus v) async {
    final fields = await _showFormDialog('Edit Virus', [
      'virusName', 'virusType', 'notes', 'location', 'owner', 'label',
    ], initial: {
      'virusName': v.virusName,
      'virusType': v.virusType,
      'notes': v.notes,
      'location': v.location,
      'owner': v.owner,
      'label': v.label,
    });
    if (fields == null) return;
    await api.updateVirus(v.id, fields);
    setState(() {});
  }

  Future<void> _addDNA() async {
    final fields = await _showFormDialog('New DNA', [
      'dnaName', 'dnaType', 'notes', 'location', 'owner', 'label',
    ]);
    if (fields == null) return;
    await api.createDNA(fields);
    setState(() {});
  }

  Future<void> _editDNA(ReagentDNA d) async {
    final fields = await _showFormDialog('Edit DNA', [
      'dnaName', 'dnaType', 'notes', 'location', 'owner', 'label',
    ], initial: {
      'dnaName': d.dnaName,
      'dnaType': d.dnaType,
      'notes': d.notes,
      'location': d.location,
      'owner': d.owner,
      'label': d.label,
    });
    if (fields == null) return;
    await api.updateDNA(d.id, fields);
    setState(() {});
  }

  Future<void> _addOligo() async {
    final fields = await _showFormDialog('New Oligo', [
      'oligoName', 'sequence', 'oligoType', 'notes', 'location', 'owner', 'label',
    ]);
    if (fields == null) return;
    await api.createOligo(fields);
    setState(() {});
  }

  Future<void> _editOligo(ReagentOligo o) async {
    final fields = await _showFormDialog('Edit Oligo', [
      'oligoName', 'sequence', 'oligoType', 'notes', 'location', 'owner', 'label',
    ], initial: {
      'oligoName': o.oligoName,
      'sequence': o.sequence,
      'oligoType': o.oligoType,
      'notes': o.notes,
      'location': o.location,
      'owner': o.owner,
      'label': o.label,
    });
    if (fields == null) return;
    await api.updateOligo(o.id, fields);
    setState(() {});
  }

  Future<void> _addChemical() async {
    final fields = await _showFormDialog('New Chemical', [
      'chemicalName', 'catalogNo', 'company', 'chemType',
      'notes', 'location', 'owner', 'label',
    ]);
    if (fields == null) return;
    await api.createChemical(fields);
    setState(() {});
  }

  Future<void> _editChemical(ReagentChemical c) async {
    final fields = await _showFormDialog('Edit Chemical', [
      'chemicalName', 'catalogNo', 'company', 'chemType',
      'notes', 'location', 'owner', 'label',
    ], initial: {
      'chemicalName': c.chemicalName,
      'catalogNo': c.catalogNo,
      'company': c.company,
      'chemType': c.chemType,
      'notes': c.notes,
      'location': c.location,
      'owner': c.owner,
      'label': c.label,
    });
    if (fields == null) return;
    await api.updateChemical(c.id, fields);
    setState(() {});
  }

  Future<void> _addMolecular() async {
    final fields = await _showFormDialog('New Molecular Reagent', [
      'mrName', 'mrType', 'notes', 'location', 'position', 'owner', 'label',
    ]);
    if (fields == null) return;
    await api.createMolecular(fields);
    setState(() {});
  }

  Future<void> _editMolecular(ReagentMolecular m) async {
    final fields = await _showFormDialog('Edit Molecular Reagent', [
      'mrName', 'mrType', 'notes', 'location', 'position', 'owner', 'label',
    ], initial: {
      'mrName': m.mrName,
      'mrType': m.mrType,
      'notes': m.notes,
      'location': m.location,
      'position': m.position,
      'owner': m.owner,
      'label': m.label,
    });
    if (fields == null) return;
    await api.updateMolecular(m.id, fields);
    setState(() {});
  }

  // =========================================================================
  // Generic form dialog
  // =========================================================================

  Future<Map<String, dynamic>?> _showFormDialog(
    String title,
    List<String> fieldKeys, {
    Map<String, String> initial = const {},
  }) async {
    final controllers = <String, TextEditingController>{};
    for (final key in fieldKeys) {
      controllers[key] = TextEditingController(text: initial[key] ?? '');
    }

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: SizedBox(
          width: 520,
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: fieldKeys.map((key) {
                return Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: TextField(
                    controller: controllers[key],
                    decoration: InputDecoration(
                      labelText: _labelFor(key),
                      isDense: true,
                      border: const OutlineInputBorder(),
                    ),
                    maxLines: key == 'notes' || key == 'sequence' ? 3 : 1,
                  ),
                );
              }).toList(),
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Save'),
          ),
        ],
      ),
    );

    if (confirmed != true) return null;

    final result = <String, dynamic>{};
    for (final key in fieldKeys) {
      result[key] = controllers[key]!.text;
    }
    return result;
  }

  String _labelFor(String key) {
    // Convert camelCase to label
    final buffer = StringBuffer();
    for (int i = 0; i < key.length; i++) {
      final ch = key[i];
      if (i == 0) {
        buffer.write(ch.toUpperCase());
      } else if (ch == ch.toUpperCase() && ch != ch.toLowerCase()) {
        buffer.write(' ');
        buffer.write(ch);
      } else {
        buffer.write(ch);
      }
    }
    return buffer.toString();
  }

  // =========================================================================
  // CSV import
  // =========================================================================

  Future<void> _importCsv(String reagentType, List<String> expectedHeaders) async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['csv'],
    );
    if (result == null || result.files.isEmpty) return;

    final filePath = result.files.single.path;
    if (filePath == null) return;

    final csvString = await File(filePath).readAsString();
    final rows = const CsvToListConverter(eol: '\n').convert(csvString);
    if (rows.isEmpty) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('CSV file is empty')),
      );
      return;
    }

    // First row = headers
    final headers = rows.first.map((e) => e.toString().trim()).toList();
    final dataRows = rows.skip(1).toList();

    if (dataRows.isEmpty) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('CSV has no data rows')),
      );
      return;
    }

    // Convert each row to a map using the headers
    final items = <Map<String, dynamic>>[];
    for (final row in dataRows) {
      final map = <String, dynamic>{};
      for (int i = 0; i < headers.length && i < row.length; i++) {
        final val = row[i].toString().trim();
        // Convert bool-like values
        if (headers[i] == 'isDepleted') {
          map[headers[i]] = val.toLowerCase() == 'true' || val == '1';
        } else if (headers[i] == 'boxId' || headers[i] == 'storageId') {
          map[headers[i]] = val.isEmpty ? null : int.tryParse(val);
        } else {
          map[headers[i]] = val;
        }
      }
      items.add(map);
    }

    try {
      final resp = await api.bulkImportReagents(reagentType, items);
      final imported = resp['imported'] ?? 0;
      final errors = (resp['errors'] as List?)?.cast<String>() ?? [];
      if (!mounted) return;
      setState(() {});
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            'Imported $imported items'
            '${errors.isNotEmpty ? ' (${errors.length} errors)' : ''}',
          ),
          duration: const Duration(seconds: 4),
        ),
      );
      if (errors.isNotEmpty) {
        showDialog(
          context: context,
          builder: (_) => AlertDialog(
            title: const Text('Import Errors'),
            content: SizedBox(
              width: 500,
              height: 300,
              child: SingleChildScrollView(
                child: Text(errors.join('\n')),
              ),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(context),
                child: const Text('OK'),
              ),
            ],
          ),
        );
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Import failed: ${e.message}')),
      );
    }
  }
}

// ===========================================================================
// Generic reagent list tab (reusable for all 7 depletable types)
// ===========================================================================

class _ReagentListTab<T> extends StatefulWidget {
  const _ReagentListTab({
    required this.api,
    required this.q,
    required this.depleted,
    required this.fetch,
    required this.nameOf,
    required this.subtitleOf,
    required this.onAdd,
    required this.onTap,
    required this.onDelete,
    required this.isDepletedOf,
    this.onImportCsv,
  });

  final ApiClient api;
  final String q;
  final bool depleted;
  final Future<List<T>> Function(String q, bool depleted) fetch;
  final String Function(T) nameOf;
  final String Function(T) subtitleOf;
  final VoidCallback onAdd;
  final Future<void> Function(T) onTap;
  final Future<void> Function(T) onDelete;
  final bool Function(T) isDepletedOf;
  final VoidCallback? onImportCsv;

  @override
  State<_ReagentListTab<T>> createState() => _ReagentListTabState<T>();
}

class _ReagentListTabState<T> extends State<_ReagentListTab<T>> {
  List<T> _items = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  @override
  void didUpdateWidget(covariant _ReagentListTab<T> oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.q != widget.q || oldWidget.depleted != widget.depleted) {
      _refresh();
    }
  }

  Future<void> _refresh() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final items = await widget.fetch(widget.q, widget.depleted);
      if (!mounted) return;
      setState(() {
        _items = items;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        if (_loading)
          const Center(child: CircularProgressIndicator())
        else if (_error != null)
          Center(child: Text('Error: $_error'))
        else if (_items.isEmpty)
          const Center(child: Text('No items'))
        else
          RefreshIndicator(
            onRefresh: _refresh,
            child: ListView.builder(
              itemCount: _items.length,
              itemBuilder: (_, i) {
                final item = _items[i];
                final depleted = widget.isDepletedOf(item);
                return ListTile(
                  title: Text(
                    widget.nameOf(item),
                    style: depleted
                        ? const TextStyle(
                            decoration: TextDecoration.lineThrough,
                            color: Colors.grey,
                          )
                        : null,
                  ),
                  subtitle: Text(widget.subtitleOf(item)),
                  trailing: depleted
                      ? const Chip(
                          label: Text('Depleted',
                              style: TextStyle(fontSize: 11)),
                          backgroundColor: Colors.orange,
                        )
                      : null,
                  onTap: () async {
                    await widget.onTap(item);
                    _refresh();
                  },
                  onLongPress: () async {
                    final confirm = await showDialog<bool>(
                      context: context,
                      builder: (_) => AlertDialog(
                        title: const Text('Mark as depleted?'),
                        content: Text(
                            'This will soft-delete "${widget.nameOf(item)}".'),
                        actions: [
                          TextButton(
                            onPressed: () => Navigator.pop(context, false),
                            child: const Text('Cancel'),
                          ),
                          FilledButton(
                            onPressed: () => Navigator.pop(context, true),
                            child: const Text('Deplete'),
                          ),
                        ],
                      ),
                    );
                    if (confirm == true) {
                      try {
                        await widget.onDelete(item);
                        _refresh();
                      } on ApiException catch (e) {
                        if (!mounted) return;
                        ScaffoldMessenger.of(context).showSnackBar(
                          SnackBar(content: Text(e.message)),
                        );
                      }
                    }
                  },
                );
              },
            ),
          ),
        Positioned(
          bottom: 16,
          right: 16,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (widget.onImportCsv != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: FloatingActionButton.small(
                    heroTag: 'reagent_import_$T',
                    onPressed: widget.onImportCsv,
                    tooltip: 'Import CSV',
                    child: const Icon(Icons.upload_file),
                  ),
                ),
              FloatingActionButton(
                heroTag: 'reagent_add_$T',
                onPressed: () {
                  widget.onAdd();
                },
                child: const Icon(Icons.add),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// ===========================================================================
// Storage tab
// ===========================================================================

class _StorageTab extends StatefulWidget {
  const _StorageTab({required this.api});
  final ApiClient api;

  @override
  State<_StorageTab> createState() => _StorageTabState();
}

class _StorageTabState extends State<_StorageTab> {
  List<ReagentStorage> _items = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final items = await widget.api.listStorage();
      if (!mounted) return;
      setState(() {
        _items = items;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
    }
  }

  Future<void> _add() async {
    final nameCtl = TextEditingController();
    final typeCtl = TextEditingController();
    final descCtl = TextEditingController();

    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('New Storage Location'),
        content: SizedBox(
          width: 400,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: nameCtl, decoration: const InputDecoration(labelText: 'Name', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: typeCtl, decoration: const InputDecoration(labelText: 'Location Type', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: descCtl, maxLines: 2, decoration: const InputDecoration(labelText: 'Description', border: OutlineInputBorder())),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Create')),
        ],
      ),
    );
    if (ok != true || nameCtl.text.trim().isEmpty) return;
    await widget.api.createStorage({
      'name': nameCtl.text.trim(),
      'locationType': typeCtl.text.trim(),
      'description': descCtl.text.trim(),
    });
    _refresh();
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        if (_loading)
          const Center(child: CircularProgressIndicator())
        else if (_items.isEmpty)
          const Center(child: Text('No storage locations'))
        else
          ListView.builder(
            itemCount: _items.length,
            itemBuilder: (_, i) {
              final s = _items[i];
              return ListTile(
                leading: const Icon(Icons.warehouse),
                title: Text(s.name),
                subtitle: Text('${s.locationType} — ${s.description}'),
              );
            },
          ),
        Positioned(
          bottom: 16,
          right: 16,
          child: FloatingActionButton(
            heroTag: 'storage_add',
            onPressed: _add,
            child: const Icon(Icons.add),
          ),
        ),
      ],
    );
  }
}

// ===========================================================================
// Box tab
// ===========================================================================

class _BoxTab extends StatefulWidget {
  const _BoxTab({required this.api, required this.q});
  final ApiClient api;
  final String q;

  @override
  State<_BoxTab> createState() => _BoxTabState();
}

class _BoxTabState extends State<_BoxTab> {
  List<ReagentBox> _items = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  @override
  void didUpdateWidget(covariant _BoxTab old) {
    super.didUpdateWidget(old);
    if (old.q != widget.q) _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final items = await widget.api.listBoxes(q: widget.q);
      if (!mounted) return;
      setState(() {
        _items = items;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
    }
  }

  Future<void> _add() async {
    final boxNoCtl = TextEditingController();
    final typeCtl = TextEditingController();
    final ownerCtl = TextEditingController();
    final labelCtl = TextEditingController();
    final locCtl = TextEditingController();

    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('New Box'),
        content: SizedBox(
          width: 400,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: boxNoCtl, decoration: const InputDecoration(labelText: 'Box No', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: typeCtl, decoration: const InputDecoration(labelText: 'Box Type', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: ownerCtl, decoration: const InputDecoration(labelText: 'Owner', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: labelCtl, decoration: const InputDecoration(labelText: 'Label', border: OutlineInputBorder())),
              const SizedBox(height: 8),
              TextField(controller: locCtl, decoration: const InputDecoration(labelText: 'Location', border: OutlineInputBorder())),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Create')),
        ],
      ),
    );
    if (ok != true) return;
    await widget.api.createBox({
      'boxNo': boxNoCtl.text.trim(),
      'boxType': typeCtl.text.trim(),
      'owner': ownerCtl.text.trim(),
      'label': labelCtl.text.trim(),
      'location': locCtl.text.trim(),
    });
    _refresh();
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        if (_loading)
          const Center(child: CircularProgressIndicator())
        else if (_items.isEmpty)
          const Center(child: Text('No boxes'))
        else
          ListView.builder(
            itemCount: _items.length,
            itemBuilder: (_, i) {
              final b = _items[i];
              return ListTile(
                leading: const Icon(Icons.inventory_2),
                title: Text('Box ${b.boxNo} — ${b.label}'),
                subtitle: Text(
                    '${b.boxType} | Owner: ${b.owner} | Loc: ${b.location}'),
              );
            },
          ),
        Positioned(
          bottom: 16,
          right: 16,
          child: FloatingActionButton(
            heroTag: 'box_add',
            onPressed: _add,
            child: const Icon(Icons.add),
          ),
        ),
      ],
    );
  }
}
