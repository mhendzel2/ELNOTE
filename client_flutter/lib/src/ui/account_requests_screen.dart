import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/sync_service.dart';

class AccountRequestsScreen extends StatefulWidget {
  const AccountRequestsScreen({super.key, required this.sync});

  final SyncService sync;

  @override
  State<AccountRequestsScreen> createState() => _AccountRequestsScreenState();
}

class _AccountRequestsScreenState extends State<AccountRequestsScreen> {
  List<Map<String, dynamic>> _pendingRequests = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final requests = await widget.sync.api.listAccountRequests(status: 'pending');
      if (!mounted) return;
      setState(() {
        _pendingRequests = requests;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _approveRequest(Map<String, dynamic> request) async {
    final requestId = request['requestId'] as String? ?? '';
    final requestType = request['requestType'] as String? ?? 'account_create';
    final email = request['email'] as String? ?? '';
    final tempPasswordCtl = TextEditingController();
    String role = 'author';

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: Text(requestType == 'password_recovery' ? 'Approve Password Reset' : 'Approve Account Request'),
          content: SizedBox(
            width: 420,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text('Email: $email'),
                const SizedBox(height: 12),
                TextField(
                  controller: tempPasswordCtl,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: 'Temporary password'),
                ),
                if (requestType == 'account_create') ...[
                  const SizedBox(height: 12),
                  DropdownButtonFormField<String>(
                    value: role,
                    decoration: const InputDecoration(labelText: 'Role'),
                    items: const [
                      DropdownMenuItem(value: 'viewer', child: Text('Viewer')),
                      DropdownMenuItem(value: 'author', child: Text('Author')),
                      DropdownMenuItem(value: 'admin', child: Text('Admin')),
                      DropdownMenuItem(value: 'owner', child: Text('Owner')),
                    ],
                    onChanged: (v) {
                      if (v != null) setDialogState(() => role = v);
                    },
                  ),
                ],
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Approve')),
          ],
        ),
      ),
    );

    if (confirmed != true || tempPasswordCtl.text.isEmpty || requestId.isEmpty) return;

    try {
      await widget.sync.api.approveAccountRequest(
        requestId: requestId,
        temporaryPassword: tempPasswordCtl.text,
        role: role,
      );
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Request approved')),
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _dismissRequest(Map<String, dynamic> request) async {
    final requestId = request['requestId'] as String? ?? '';
    if (requestId.isEmpty) return;

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Dismiss Request'),
        content: const Text('Dismiss this pending request?'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(_, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(_, true), child: const Text('Dismiss')),
        ],
      ),
    );

    if (confirmed != true) return;

    try {
      await widget.sync.api.dismissAccountRequest(requestId: requestId);
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Request dismissed')),
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }

    if (_pendingRequests.isEmpty) {
      return const Center(child: Text('No pending account or password reset requests'));
    }

    return Scaffold(
      appBar: AppBar(
        title: const Text('Approvals'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: _refresh,
            tooltip: 'Refresh',
          ),
        ],
      ),
      body: ListView.builder(
        padding: const EdgeInsets.all(16),
        itemCount: _pendingRequests.length,
        itemBuilder: (context, index) {
          final request = _pendingRequests[index];
          final requestType = request['requestType'] as String? ?? '';
          final username = request['username'] as String? ?? '';
          final email = request['email'] as String? ?? '';
          final note = request['note'] as String? ?? '';

          return Card(
            child: ListTile(
              leading: Icon(
                requestType == 'password_recovery' ? Icons.lock_reset : Icons.person_add_alt,
              ),
              title: Text('$username ($email)'),
              subtitle: Text(
                '${requestType == 'password_recovery' ? 'Password reset request' : 'Account request'}${note.isNotEmpty ? '\n$note' : ''}',
              ),
              trailing: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  IconButton(
                    icon: const Icon(Icons.check_circle, color: Colors.green),
                    tooltip: 'Approve',
                    onPressed: () => _approveRequest(request),
                  ),
                  IconButton(
                    icon: const Icon(Icons.cancel, color: Colors.red),
                    tooltip: 'Dismiss',
                    onPressed: () => _dismissRequest(request),
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}
