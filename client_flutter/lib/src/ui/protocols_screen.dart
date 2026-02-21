import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/local_database.dart';
import '../data/sync_service.dart';

class ProtocolsScreen extends StatefulWidget {
  const ProtocolsScreen({super.key, required this.db, required this.sync});

  final LocalDatabase db;
  final SyncService sync;

  @override
  State<ProtocolsScreen> createState() => _ProtocolsScreenState();
}

class _ProtocolsScreenState extends State<ProtocolsScreen> {
  List<Map<String, Object?>> _protocols = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      await widget.sync.refreshProtocols();
    } catch (_) {}
    final rows = await widget.db.listLocalProtocols();
    if (!mounted) return;
    setState(() {
      _protocols = rows;
      _loading = false;
    });
  }

  Future<void> _createProtocol() async {
    final titleCtl = TextEditingController();
    final descCtl = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('New Protocol'),
        content: SizedBox(
          width: 480,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: titleCtl, decoration: const InputDecoration(labelText: 'Title')),
              const SizedBox(height: 12),
              TextField(controller: descCtl, maxLines: 4, decoration: const InputDecoration(labelText: 'Description')),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Create')),
        ],
      ),
    );

    if (confirmed != true || titleCtl.text.trim().isEmpty) return;

    try {
      await widget.sync.api.createProtocol(
        title: titleCtl.text.trim(),
        description: descCtl.text.trim(),
      );
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const Center(child: CircularProgressIndicator());

    return Scaffold(
      body: _protocols.isEmpty
          ? const Center(child: Text('No protocols yet'))
          : ListView.separated(
              itemCount: _protocols.length,
              separatorBuilder: (_, __) => const Divider(height: 1),
              itemBuilder: (context, i) {
                final p = _protocols[i];
                final status = p['status'] as String? ?? 'draft';
                return ListTile(
                  leading: Icon(
                    status == 'active' ? Icons.check_circle : Icons.article_outlined,
                    color: status == 'active' ? Colors.green : null,
                  ),
                  title: Text(p['title'] as String? ?? ''),
                  subtitle: Text('Status: ${status.toUpperCase()}'),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () async {
                    await Navigator.push(
                      context,
                      MaterialPageRoute<void>(
                        builder: (_) => _ProtocolDetailScreen(
                          protocolId: p['protocol_id'] as String,
                          title: p['title'] as String? ?? '',
                          api: widget.sync.api,
                        ),
                      ),
                    );
                    await _refresh();
                  },
                );
              },
            ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _createProtocol,
        icon: const Icon(Icons.add),
        label: const Text('New Protocol'),
      ),
    );
  }
}

class _ProtocolDetailScreen extends StatefulWidget {
  const _ProtocolDetailScreen({required this.protocolId, required this.title, required this.api});
  final String protocolId;
  final String title;
  final ApiClient api;

  @override
  State<_ProtocolDetailScreen> createState() => _ProtocolDetailScreenState();
}

class _ProtocolDetailScreenState extends State<_ProtocolDetailScreen> {
  Map<String, dynamic>? _protocol;
  List<dynamic> _versions = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final proto = await widget.api.getProtocol(widget.protocolId);
      final versions = await widget.api.listProtocolVersions(widget.protocolId);
      if (!mounted) return;
      setState(() {
        _protocol = proto;
        _versions = versions;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _publishVersion() async {
    final bodyCtl = TextEditingController();
    final changeLogCtl = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Publish Version'),
        content: SizedBox(
          width: 480,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: bodyCtl, maxLines: 8, decoration: const InputDecoration(labelText: 'Protocol body (full text)')),
              const SizedBox(height: 12),
              TextField(controller: changeLogCtl, maxLines: 3, decoration: const InputDecoration(labelText: 'Change log')),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Publish')),
        ],
      ),
    );

    if (confirmed != true || bodyCtl.text.trim().isEmpty) return;

    try {
      await widget.api.publishProtocolVersion(
        protocolId: widget.protocolId,
        body: bodyCtl.text.trim(),
        changeLog: changeLogCtl.text.trim(),
      );
      await _load();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(widget.title)),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _publishVersion,
        icon: const Icon(Icons.publish),
        label: const Text('Publish Version'),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(16),
              children: [
                if (_protocol != null) ...[
                  Card(
                    child: Padding(
                      padding: const EdgeInsets.all(12),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text('Status: ${(_protocol!['status'] as String? ?? 'draft').toUpperCase()}',
                              style: const TextStyle(fontWeight: FontWeight.bold)),
                          const SizedBox(height: 8),
                          Text(_protocol!['description'] as String? ?? ''),
                        ],
                      ),
                    ),
                  ),
                ],
                const SizedBox(height: 12),
                Text('Versions (${_versions.length})', style: const TextStyle(fontWeight: FontWeight.bold)),
                const SizedBox(height: 6),
                if (_versions.isEmpty) const Text('No published versions'),
                ..._versions.map((v) {
                  final ver = v as Map<String, dynamic>;
                  return Card(
                    child: ExpansionTile(
                      title: Text('v${ver['versionNumber'] ?? '?'}'),
                      subtitle: Text(ver['changeLog'] as String? ?? ''),
                      children: [
                        Padding(
                          padding: const EdgeInsets.all(12),
                          child: SelectableText(ver['body'] as String? ?? ''),
                        ),
                      ],
                    ),
                  );
                }),
              ],
            ),
    );
  }
}
