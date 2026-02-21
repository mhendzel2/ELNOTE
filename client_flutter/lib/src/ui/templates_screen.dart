import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/local_database.dart';
import '../data/sync_service.dart';

class TemplatesScreen extends StatefulWidget {
  const TemplatesScreen({super.key, required this.db, required this.sync});
  final LocalDatabase db;
  final SyncService sync;

  @override
  State<TemplatesScreen> createState() => _TemplatesScreenState();
}

class _TemplatesScreenState extends State<TemplatesScreen> {
  List<Map<String, Object?>> _templates = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      await widget.sync.refreshTemplates();
    } catch (_) {}
    final rows = await widget.db.listLocalTemplates();
    if (!mounted) return;
    setState(() {
      _templates = rows;
      _loading = false;
    });
  }

  Future<void> _createTemplate() async {
    final titleCtl = TextEditingController();
    final descCtl = TextEditingController();
    final bodyCtl = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('New Template'),
        content: SizedBox(
          width: 520,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: titleCtl, decoration: const InputDecoration(labelText: 'Title')),
              const SizedBox(height: 12),
              TextField(controller: descCtl, maxLines: 2, decoration: const InputDecoration(labelText: 'Description')),
              const SizedBox(height: 12),
              TextField(controller: bodyCtl, maxLines: 6, decoration: const InputDecoration(labelText: 'Body template')),
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
      await widget.sync.api.createTemplate(
        title: titleCtl.text.trim(),
        description: descCtl.text.trim(),
        bodyTemplate: bodyCtl.text.trim(),
        sections: [],
      );
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _createExperimentFromTemplate(Map<String, Object?> template) async {
    final titleCtl = TextEditingController(text: template['title'] as String? ?? '');

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('New Experiment from Template'),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(controller: titleCtl, decoration: const InputDecoration(labelText: 'Experiment title')),
              const SizedBox(height: 12),
              Text(
                'Body template will be used as the initial content.',
                style: TextStyle(color: Colors.grey.shade600),
              ),
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
      await widget.sync.api.createFromTemplate(
        templateId: template['template_id'] as String,
        title: titleCtl.text.trim(),
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Experiment created from template')),
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const Center(child: CircularProgressIndicator());

    return Scaffold(
      body: _templates.isEmpty
          ? const Center(child: Text('No templates yet'))
          : ListView.separated(
              itemCount: _templates.length,
              separatorBuilder: (_, __) => const Divider(height: 1),
              itemBuilder: (context, i) {
                final t = _templates[i];
                return ListTile(
                  leading: const Icon(Icons.description_outlined),
                  title: Text(t['title'] as String? ?? ''),
                  subtitle: Text(t['description'] as String? ?? ''),
                  trailing: PopupMenuButton<String>(
                    onSelected: (value) {
                      if (value == 'create') {
                        _createExperimentFromTemplate(t);
                      }
                    },
                    itemBuilder: (_) => [
                      const PopupMenuItem(value: 'create', child: Text('Create experiment from template')),
                    ],
                  ),
                );
              },
            ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _createTemplate,
        icon: const Icon(Icons.add),
        label: const Text('New Template'),
      ),
    );
  }
}
