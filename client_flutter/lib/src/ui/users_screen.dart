import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/sync_service.dart';

class UsersScreen extends StatefulWidget {
  const UsersScreen({super.key, required this.sync});
  final SyncService sync;

  @override
  State<UsersScreen> createState() => _UsersScreenState();
}

class _UsersScreenState extends State<UsersScreen> {
  List<Map<String, dynamic>> _users = [];
  List<Map<String, dynamic>> _pendingRequests = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    try {
      final users = await widget.sync.api.listUsers();
      final requests = await widget.sync.api.listAccountRequests(status: 'pending');
      if (!mounted) return;
      setState(() {
        _users = users;
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

  Future<void> _createUser() async {
    final emailCtl = TextEditingController();
    final passwordCtl = TextEditingController();
    String role = 'author';

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: const Text('Create User'),
          content: SizedBox(
            width: 400,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: emailCtl,
                  decoration: const InputDecoration(labelText: 'Username / Email'),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: passwordCtl,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: 'Password'),
                ),
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
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Create')),
          ],
        ),
      ),
    );

    if (confirmed != true || emailCtl.text.trim().isEmpty || passwordCtl.text.isEmpty) return;

    try {
      await widget.sync.api.createUser(
        email: emailCtl.text.trim(),
        password: passwordCtl.text,
        role: role,
      );
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('User created')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _changeRole(String userId, String currentRole) async {
    String role = currentRole;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: const Text('Change Role'),
          content: DropdownButtonFormField<String>(
            value: role,
            decoration: const InputDecoration(labelText: 'New Role'),
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
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Save')),
          ],
        ),
      ),
    );

    if (confirmed != true) return;

    try {
      await widget.sync.api.updateUser(userId: userId, role: role);
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Role updated')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _deleteUser(String userId, String email) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Delete User'),
        content: Text('Are you sure you want to delete "$email"?'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(_, false), child: const Text('Cancel')),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: Colors.red),
            onPressed: () => Navigator.pop(_, true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );

    if (confirmed != true) return;

    try {
      await widget.sync.api.deleteUser(userId);
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('"$email" deleted')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _resetLabAdmin() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Reset LabAdmin'),
        content: const Text(
          'This will reset the LabAdmin password to the default value (CCI#3341).\n\n'
          'Use this if the LabAdmin password has been lost.',
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(_, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(_, true), child: const Text('Reset')),
        ],
      ),
    );

    if (confirmed != true) return;

    try {
      await widget.sync.api.resetLabAdmin();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('LabAdmin password reset to default')),
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _changePassword(String userId, String email) async {
    final currentCtl = TextEditingController();
    final nextCtl = TextEditingController();
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text('Change Password ($email)'),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: currentCtl,
                obscureText: true,
                decoration: const InputDecoration(labelText: 'Current password'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: nextCtl,
                obscureText: true,
                decoration: const InputDecoration(labelText: 'New password'),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Change')),
        ],
      ),
    );
    if (confirmed != true || currentCtl.text.isEmpty || nextCtl.text.isEmpty) return;
    try {
      await widget.sync.api.changePassword(
        userId: userId,
        currentPassword: currentCtl.text,
        newPassword: nextCtl.text,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Password updated')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Color _roleColor(String role) {
    switch (role) {
      case 'owner':
        return Colors.purple;
      case 'admin':
        return Colors.blue;
      case 'author':
        return Colors.green;
      case 'viewer':
        return Colors.grey;
      default:
        return Colors.grey;
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }

    return Scaffold(
      floatingActionButton: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          FloatingActionButton.small(
            heroTag: 'reset',
            onPressed: _resetLabAdmin,
            tooltip: 'Reset LabAdmin to default password',
            child: const Icon(Icons.restart_alt),
          ),
          const SizedBox(height: 12),
          FloatingActionButton.extended(
            heroTag: 'create',
            onPressed: _createUser,
            icon: const Icon(Icons.person_add),
            label: const Text('New User'),
          ),
        ],
      ),
      body: _users.isEmpty
          ? const Center(child: Text('No users found'))
          : ListView.builder(
              padding: const EdgeInsets.all(16),
              itemCount: _users.length + 1,
              itemBuilder: (context, index) {
                if (index == 0) {
                  return Card(
                    margin: const EdgeInsets.only(bottom: 16),
                    child: Padding(
                      padding: const EdgeInsets.all(12),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Pending approvals (${_pendingRequests.length})',
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                          const SizedBox(height: 8),
                          if (_pendingRequests.isEmpty)
                            const Text('No pending account/password reset requests')
                          else
                            ..._pendingRequests.map((request) {
                              final requestType = request['requestType'] as String? ?? '';
                              final username = request['username'] as String? ?? '';
                              final email = request['email'] as String? ?? '';
                              final note = request['note'] as String? ?? '';
                              return ListTile(
                                contentPadding: EdgeInsets.zero,
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
                              );
                            }),
                        ],
                      ),
                    ),
                  );
                }

                final user = _users[index - 1];
                final userId = user['userId'] as String? ?? '';
                final email = user['email'] as String? ?? '';
                final role = user['role'] as String? ?? '';
                final isDefault = user['isDefaultAdmin'] == true;

                return Card(
                  child: ListTile(
                    leading: CircleAvatar(
                      backgroundColor: _roleColor(role),
                      child: Text(
                        email.isNotEmpty ? email[0].toUpperCase() : '?',
                        style: const TextStyle(color: Colors.white),
                      ),
                    ),
                    title: Row(
                      children: [
                        Text(email),
                        if (isDefault) ...[
                          const SizedBox(width: 8),
                          Chip(
                            label: const Text('Default'),
                            labelStyle: const TextStyle(fontSize: 11),
                            padding: EdgeInsets.zero,
                            visualDensity: VisualDensity.compact,
                            backgroundColor: Colors.amber.shade100,
                          ),
                        ],
                      ],
                    ),
                    subtitle: Text('Role: $role'),
                    trailing: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        IconButton(
                          icon: const Icon(Icons.edit),
                          tooltip: 'Change role',
                          onPressed: () => _changeRole(userId, role),
                        ),
                        IconButton(
                          icon: const Icon(Icons.password),
                          tooltip: 'Change password',
                          onPressed: () => _changePassword(userId, email),
                        ),
                        if (!isDefault)
                          IconButton(
                            icon: const Icon(Icons.delete, color: Colors.red),
                            tooltip: 'Delete user',
                            onPressed: () => _deleteUser(userId, email),
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
