import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/local_database.dart';
import '../data/sync_service.dart';
import '../models/models.dart';

class ElnoteApplication extends StatefulWidget {
  const ElnoteApplication({
    super.key,
    required this.db,
  });

  final LocalDatabase db;

  @override
  State<ElnoteApplication> createState() => _ElnoteApplicationState();
}

class _ElnoteApplicationState extends State<ElnoteApplication> {
  AuthSession? _session;
  ApiClient? _api;
  SyncService? _sync;

  final _baseUrlController = TextEditingController(text: 'http://localhost:8080');
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  final _deviceController = TextEditingController(text: 'tablet-1');

  bool _loggingIn = false;
  String? _loginError;

  @override
  void dispose() {
    _baseUrlController.dispose();
    _emailController.dispose();
    _passwordController.dispose();
    _deviceController.dispose();
    _sync?.dispose();
    super.dispose();
  }

  Future<void> _login() async {
    setState(() {
      _loggingIn = true;
      _loginError = null;
    });

    try {
      final api = ApiClient(baseUrl: _baseUrlController.text.trim());
      final session = await api.login(
        email: _emailController.text.trim(),
        password: _passwordController.text,
        deviceName: _deviceController.text.trim(),
      );
      api.accessToken = session.accessToken;

      final sync = SyncService(db: widget.db, api: api);
      await sync.syncNow();
      await sync.startWebSocket();

      if (!mounted) {
        return;
      }

      setState(() {
        _session = session;
        _api = api;
        _sync = sync;
      });
    } on ApiException catch (e) {
      setState(() {
        _loginError = e.message;
      });
    } catch (e) {
      setState(() {
        _loginError = e.toString();
      });
    } finally {
      if (mounted) {
        setState(() {
          _loggingIn = false;
        });
      }
    }
  }

  Future<void> _logout() async {
    await _sync?.dispose();
    if (!mounted) {
      return;
    }

    setState(() {
      _session = null;
      _api = null;
      _sync = null;
      _loginError = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      title: 'ELNOTE Offline MVP',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF005F73)),
        useMaterial3: true,
      ),
      home: _session == null
          ? _LoginScreen(
              baseUrlController: _baseUrlController,
              emailController: _emailController,
              passwordController: _passwordController,
              deviceController: _deviceController,
              loggingIn: _loggingIn,
              loginError: _loginError,
              onLogin: _login,
            )
          : _WorkspaceScreen(
              db: widget.db,
              sync: _sync!,
              onLogout: _logout,
            ),
    );
  }
}

class _LoginScreen extends StatelessWidget {
  const _LoginScreen({
    required this.baseUrlController,
    required this.emailController,
    required this.passwordController,
    required this.deviceController,
    required this.loggingIn,
    required this.loginError,
    required this.onLogin,
  });

  final TextEditingController baseUrlController;
  final TextEditingController emailController;
  final TextEditingController passwordController;
  final TextEditingController deviceController;
  final bool loggingIn;
  final String? loginError;
  final Future<void> Function() onLogin;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  'ELNOTE',
                  textAlign: TextAlign.center,
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 24),
                TextField(
                  controller: baseUrlController,
                  decoration: const InputDecoration(labelText: 'API Base URL'),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: emailController,
                  decoration: const InputDecoration(labelText: 'Email'),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: passwordController,
                  decoration: const InputDecoration(labelText: 'Password'),
                  obscureText: true,
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: deviceController,
                  decoration: const InputDecoration(labelText: 'Device name'),
                ),
                const SizedBox(height: 16),
                FilledButton(
                  onPressed: loggingIn ? null : onLogin,
                  child: Text(loggingIn ? 'Signing in...' : 'Sign in'),
                ),
                if (loginError != null) ...[
                  const SizedBox(height: 12),
                  Text(
                    loginError!,
                    style: const TextStyle(color: Colors.red),
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _WorkspaceScreen extends StatefulWidget {
  const _WorkspaceScreen({
    required this.db,
    required this.sync,
    required this.onLogout,
  });

  final LocalDatabase db;
  final SyncService sync;
  final Future<void> Function() onLogout;

  @override
  State<_WorkspaceScreen> createState() => _WorkspaceScreenState();
}

class _WorkspaceScreenState extends State<_WorkspaceScreen> {
  List<ExperimentRecord> _experiments = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() {
      _loading = true;
    });

    final experiments = await widget.db.listExperiments();

    if (!mounted) {
      return;
    }

    setState(() {
      _experiments = experiments;
      _loading = false;
    });
  }

  Future<void> _syncNow() async {
    await widget.sync.syncNow();
    await _refresh();
  }

  Future<void> _createExperiment() async {
    final titleController = TextEditingController();
    final bodyController = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Create experiment'),
        content: SizedBox(
          width: 480,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: titleController,
                decoration: const InputDecoration(labelText: 'Title'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: bodyController,
                maxLines: 6,
                decoration: const InputDecoration(
                  labelText: 'Original entry',
                  alignLabelWithHint: true,
                ),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Queue create'),
          ),
        ],
      ),
    );

    if (confirmed != true) {
      return;
    }

    await widget.db.createLocalExperimentDraft(
      title: titleController.text,
      originalBody: bodyController.text,
    );

    await _syncNow();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('ELNOTE Workspace'),
        actions: [
          IconButton(
            icon: const Icon(Icons.sync),
            onPressed: _syncNow,
            tooltip: 'Sync now',
          ),
          IconButton(
            icon: const Icon(Icons.logout),
            onPressed: widget.onLogout,
            tooltip: 'Logout',
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _createExperiment,
        icon: const Icon(Icons.add),
        label: const Text('New Experiment'),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _experiments.isEmpty
              ? const Center(child: Text('No experiments yet'))
              : ListView.separated(
                  itemCount: _experiments.length,
                  separatorBuilder: (_, __) => const Divider(height: 1),
                  itemBuilder: (context, index) {
                    final experiment = _experiments[index];
                    return ListTile(
                      title: Text(experiment.title),
                      subtitle: Text(
                        '${experiment.status.toUpperCase()} | ${experiment.effectiveBody}',
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                      trailing: const Icon(Icons.chevron_right),
                      onTap: () async {
                        await Navigator.of(context).push(
                          MaterialPageRoute<void>(
                            builder: (_) => _ExperimentDetailScreen(
                              db: widget.db,
                              sync: widget.sync,
                              experimentLocalId: experiment.localId,
                            ),
                          ),
                        );
                        await _refresh();
                      },
                    );
                  },
                ),
    );
  }
}

class _ExperimentDetailScreen extends StatefulWidget {
  const _ExperimentDetailScreen({
    required this.db,
    required this.sync,
    required this.experimentLocalId,
  });

  final LocalDatabase db;
  final SyncService sync;
  final String experimentLocalId;

  @override
  State<_ExperimentDetailScreen> createState() => _ExperimentDetailScreenState();
}

class _ExperimentDetailScreenState extends State<_ExperimentDetailScreen> {
  ExperimentRecord? _experiment;
  List<EntryRecord> _entries = const [];
  List<CommentRecord> _comments = const [];
  List<ProposalRecord> _proposals = const [];
  List<ConflictArtifact> _conflicts = const [];

  final _addendumController = TextEditingController();
  final _commentController = TextEditingController();
  final _proposalTitleController = TextEditingController();
  final _proposalBodyController = TextEditingController();

  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  @override
  void dispose() {
    _addendumController.dispose();
    _commentController.dispose();
    _proposalTitleController.dispose();
    _proposalBodyController.dispose();
    super.dispose();
  }

  Future<void> _refresh() async {
    setState(() {
      _loading = true;
    });

    final experiments = await widget.db.listExperiments();
    if (experiments.isEmpty) {
      if (!mounted) {
        return;
      }
      setState(() {
        _experiment = null;
        _entries = const [];
        _comments = const [];
        _proposals = const [];
        _conflicts = const [];
        _loading = false;
      });
      return;
    }

    final experiment = experiments.firstWhere(
      (item) => item.localId == widget.experimentLocalId,
      orElse: () => experiments.first,
    );

    final entries = await widget.db.listEntries(experiment.localId);
    final comments = await widget.db.listComments(experiment.localId);
    final proposals = await widget.db.listProposals(experiment.localId);
    final conflicts = (await widget.db.listConflicts())
        .where((item) => item.experimentId == (experiment.serverId ?? ''))
        .toList(growable: false);

    if (!mounted) {
      return;
    }

    setState(() {
      _experiment = experiment;
      _entries = entries;
      _comments = comments;
      _proposals = proposals;
      _conflicts = conflicts;
      _loading = false;
    });
  }

  Future<void> _queueAddendum() async {
    final experiment = _experiment;
    if (experiment == null || _addendumController.text.trim().isEmpty) {
      return;
    }

    await widget.db.queueAddendum(
      experimentLocalId: experiment.localId,
      body: _addendumController.text.trim(),
    );
    _addendumController.clear();

    await widget.sync.syncNow();
    await _refresh();
  }

  Future<void> _queueComment() async {
    final experiment = _experiment;
    if (experiment == null || _commentController.text.trim().isEmpty) {
      return;
    }

    await widget.db.queueComment(
      experimentLocalId: experiment.localId,
      body: _commentController.text.trim(),
    );
    _commentController.clear();

    await widget.sync.syncNow();
    await _refresh();
  }

  Future<void> _queueProposal() async {
    final experiment = _experiment;
    if (experiment == null || _proposalTitleController.text.trim().isEmpty || _proposalBodyController.text.trim().isEmpty) {
      return;
    }

    await widget.db.queueProposal(
      sourceExperimentLocalId: experiment.localId,
      title: _proposalTitleController.text.trim(),
      body: _proposalBodyController.text.trim(),
    );
    _proposalTitleController.clear();
    _proposalBodyController.clear();

    await widget.sync.syncNow();
    await _refresh();
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(
        body: Center(child: CircularProgressIndicator()),
      );
    }
    if (_experiment == null) {
      return const Scaffold(
        body: Center(child: Text('Experiment not available')),
      );
    }

    final experiment = _experiment!;

    return Scaffold(
      appBar: AppBar(
        title: Text(experiment.title),
        actions: [
          IconButton(
            icon: const Icon(Icons.sync),
            onPressed: () async {
              await widget.sync.syncNow();
              await _refresh();
            },
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Status: ${experiment.status}'),
                  const SizedBox(height: 8),
                  const Text('Effective content'),
                  const SizedBox(height: 4),
                  Text(experiment.effectiveBody),
                ],
              ),
            ),
          ),
          const SizedBox(height: 12),
          const Text('Immutable history', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          ..._entries.map(
            (entry) => Card(
              child: ListTile(
                title: Text(entry.entryType.toUpperCase()),
                subtitle: Text(entry.body),
                trailing: Text('${entry.createdAt.hour}:${entry.createdAt.minute.toString().padLeft(2, '0')}'),
              ),
            ),
          ),
          const SizedBox(height: 12),
          const Text('Add correction (addendum only)', style: TextStyle(fontWeight: FontWeight.bold)),
          TextField(
            controller: _addendumController,
            maxLines: 4,
            decoration: const InputDecoration(
              hintText: 'Describe correction as an immutable addendum',
              alignLabelWithHint: true,
            ),
          ),
          const SizedBox(height: 8),
          FilledButton(
            onPressed: _queueAddendum,
            child: const Text('Queue Addendum'),
          ),
          const SizedBox(height: 12),
          const Text('Comments', style: TextStyle(fontWeight: FontWeight.bold)),
          ..._comments.map(
            (comment) => ListTile(
              dense: true,
              title: Text(comment.body),
              subtitle: Text(comment.createdAt.toIso8601String()),
            ),
          ),
          TextField(
            controller: _commentController,
            maxLines: 2,
            decoration: const InputDecoration(hintText: 'Admin comment'),
          ),
          const SizedBox(height: 8),
          OutlinedButton(
            onPressed: _queueComment,
            child: const Text('Queue Comment'),
          ),
          const SizedBox(height: 12),
          const Text('Proposals', style: TextStyle(fontWeight: FontWeight.bold)),
          ..._proposals.map(
            (proposal) => ListTile(
              dense: true,
              title: Text(proposal.title),
              subtitle: Text(proposal.body),
            ),
          ),
          TextField(
            controller: _proposalTitleController,
            decoration: const InputDecoration(hintText: 'Proposal title'),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _proposalBodyController,
            maxLines: 3,
            decoration: const InputDecoration(hintText: 'Proposal details'),
          ),
          const SizedBox(height: 8),
          OutlinedButton(
            onPressed: _queueProposal,
            child: const Text('Queue Proposal'),
          ),
          const SizedBox(height: 12),
          const Text('Conflict artifacts', style: TextStyle(fontWeight: FontWeight.bold)),
          if (_conflicts.isEmpty)
            const Padding(
              padding: EdgeInsets.only(top: 6),
              child: Text('No recorded conflicts for this experiment'),
            )
          else
            ..._conflicts.map(
              (conflict) => Card(
                child: ListTile(
                  title: Text('Conflict ${conflict.conflictArtifactId}'),
                  subtitle: Text(
                    'Client base: ${conflict.clientBaseEntryId ?? '-'}\nServer latest: ${conflict.serverLatestEntryId ?? '-'}',
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
