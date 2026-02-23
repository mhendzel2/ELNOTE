import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/sync_service.dart';

class OpsDashboardScreen extends StatefulWidget {
  const OpsDashboardScreen({super.key, required this.sync});

  final SyncService sync;

  @override
  State<OpsDashboardScreen> createState() => _OpsDashboardScreenState();
}

class _OpsDashboardScreenState extends State<OpsDashboardScreen> {
  Map<String, dynamic>? _dashboard;
  Map<String, dynamic>? _audit;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final dashboard = await widget.sync.api.getOpsDashboard();
      final audit = await widget.sync.api.verifyAuditChain();
      if (!mounted) return;
      setState(() {
        _dashboard = dashboard;
        _audit = audit;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(e.message)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }

    final d = _dashboard ?? const <String, dynamic>{};
    final audit = _audit ?? const <String, dynamic>{};

    return Scaffold(
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Card(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Row(
                  children: [
                    Icon(
                      (audit['valid'] == true) ? Icons.verified : Icons.warning_amber,
                      color: (audit['valid'] == true) ? Colors.green : Colors.orange,
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        'Audit Chain: ${(audit['valid'] == true) ? 'VALID' : 'CHECK FAILED'}',
                        style: const TextStyle(fontWeight: FontWeight.bold),
                      ),
                    ),
                    if (audit['checkedEvents'] != null)
                      Text('Events: ${audit['checkedEvents']}'),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 12),
            _metricTile('Auth logins (24h)', d['authLogin24h']),
            _metricTile('Auth refreshes (24h)', d['authRefresh24h']),
            _metricTile('Auth logouts (24h)', d['authLogout24h']),
            _metricTile('Sync events (24h)', d['syncEvents24h']),
            _metricTile('Sync conflicts (24h)', d['syncConflicts24h']),
            _metricTile('Attachments initiated (24h)', d['attachmentInitiated24h']),
            _metricTile('Attachments completed (24h)', d['attachmentCompleted24h']),
            _metricTile('Reconcile runs (24h)', d['reconcileRuns24h']),
            _metricTile('Unresolved findings', d['reconcileFindingsUnresolved']),
            _metricTile('Missing object findings', d['reconcileMissingObjectUnresolved']),
            _metricTile('Orphan object findings', d['reconcileOrphanObjectUnresolved']),
            _metricTile('Integrity mismatch findings', d['reconcileIntegrityMismatchUnresolved']),
            _metricTile('Audit events (24h)', d['auditEvents24h']),
          ],
        ),
      ),
    );
  }

  Widget _metricTile(String label, dynamic value) {
    return Card(
      child: ListTile(
        title: Text(label),
        trailing: Text(
          '${value ?? 0}',
          style: const TextStyle(fontWeight: FontWeight.bold),
        ),
      ),
    );
  }
}
