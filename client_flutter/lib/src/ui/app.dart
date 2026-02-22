import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:csv/csv.dart';
import 'package:file_picker/file_picker.dart';
import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:pdf/widgets.dart' as pw;
import 'package:printing/printing.dart';

import '../data/api_client.dart';
import '../data/local_database.dart';
import '../data/sync_service.dart';
import '../models/models.dart';
import 'notifications_screen.dart';
import 'ops_dashboard_screen.dart';
import 'protocols_screen.dart';
import 'reagents_screen.dart';
import 'search_screen.dart';
import 'templates_screen.dart';
import 'users_screen.dart';

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
  int _navIndex = 0;
  int _unreadCount = 0;

  @override
  void initState() {
    super.initState();
    _refreshUnread();
  }

  Future<void> _refreshUnread() async {
    final count = await widget.db.countUnreadNotifications();
    if (!mounted) return;
    setState(() => _unreadCount = count);
  }

  Future<void> _syncNow() async {
    await widget.sync.syncNow();
    await _refreshUnread();
  }

  @override
  Widget build(BuildContext context) {
    final destinations = <NavigationRailDestination>[
      const NavigationRailDestination(icon: Icon(Icons.folder), label: Text('Projects')),
      const NavigationRailDestination(icon: Icon(Icons.article), label: Text('Protocols')),
      const NavigationRailDestination(icon: Icon(Icons.description), label: Text('Templates')),
      const NavigationRailDestination(icon: Icon(Icons.search), label: Text('Search')),
      NavigationRailDestination(
        icon: Badge(
          isLabelVisible: _unreadCount > 0,
          label: Text('$_unreadCount'),
          child: const Icon(Icons.notifications),
        ),
        label: const Text('Notifications'),
      ),
      const NavigationRailDestination(icon: Icon(Icons.inventory_2), label: Text('Reagents')),
      const NavigationRailDestination(icon: Icon(Icons.people), label: Text('Users')),
      const NavigationRailDestination(icon: Icon(Icons.monitor_heart), label: Text('Ops')),
    ];

    Widget body;
    switch (_navIndex) {
      case 1:
        body = ProtocolsScreen(db: widget.db, sync: widget.sync);
        break;
      case 2:
        body = TemplatesScreen(db: widget.db, sync: widget.sync);
        break;
      case 3:
        body = SearchScreen(sync: widget.sync);
        break;
      case 4:
        body = NotificationsScreen(db: widget.db, sync: widget.sync);
        break;
      case 5:
        body = ReagentsScreen(db: widget.db, sync: widget.sync);
        break;
      case 6:
        body = UsersScreen(sync: widget.sync);
        break;
      case 7:
        body = OpsDashboardScreen(sync: widget.sync);
        break;
      default:
        body = _ProjectsBody(db: widget.db, sync: widget.sync);
    }

    return Scaffold(
      appBar: AppBar(
        title: const Text('ELNOTE Workspace'),
        actions: [
          IconButton(icon: const Icon(Icons.sync), onPressed: _syncNow, tooltip: 'Sync now'),
          IconButton(icon: const Icon(Icons.logout), onPressed: widget.onLogout, tooltip: 'Logout'),
        ],
      ),
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _navIndex,
            onDestinationSelected: (index) {
              setState(() => _navIndex = index);
              if (index == 4) _refreshUnread();
            },
            labelType: NavigationRailLabelType.all,
            destinations: destinations,
          ),
          const VerticalDivider(thickness: 1, width: 1),
          Expanded(child: body),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Projects body (index 0 in NavigationRail)
// ---------------------------------------------------------------------------

class _ProjectsBody extends StatefulWidget {
  const _ProjectsBody({required this.db, required this.sync});

  final LocalDatabase db;
  final SyncService sync;

  @override
  State<_ProjectsBody> createState() => _ProjectsBodyState();
}

class _ProjectsBodyState extends State<_ProjectsBody> {
  List<Map<String, dynamic>> _projects = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final projects = await widget.sync.api.listProjects();
      if (!mounted) return;
      setState(() {
        _projects = projects;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load projects: $e')),
      );
    }
  }

  Future<void> _createProject() async {
    final titleCtl = TextEditingController();
    final descCtl = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Create Project'),
        content: SizedBox(
          width: 480,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: titleCtl,
                decoration: const InputDecoration(labelText: 'Title'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: descCtl,
                maxLines: 3,
                decoration: const InputDecoration(
                  labelText: 'Description',
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
            child: const Text('Create'),
          ),
        ],
      ),
    );
    if (confirmed != true || titleCtl.text.trim().isEmpty) return;

    try {
      await widget.sync.api.createProject(
        title: titleCtl.text.trim(),
        description: descCtl.text.trim(),
      );
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(e.message)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _createProject,
        icon: const Icon(Icons.create_new_folder),
        label: const Text('New Project'),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _projects.isEmpty
              ? const Center(child: Text('No projects yet'))
              : RefreshIndicator(
                  onRefresh: _refresh,
                  child: ListView.separated(
                    itemCount: _projects.length,
                    separatorBuilder: (_, __) => const Divider(height: 1),
                    itemBuilder: (context, index) {
                      final p = _projects[index];
                      final status = p['status'] as String? ?? '';
                      return ListTile(
                        leading: Icon(
                          status == 'archived' ? Icons.archive : Icons.folder_open,
                          color: status == 'archived' ? Colors.grey : null,
                        ),
                        title: Text(p['title'] as String? ?? '(Untitled)'),
                        subtitle: Text(
                          p['description'] as String? ?? '',
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        trailing: const Icon(Icons.chevron_right),
                        onTap: () async {
                          await Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (_) => _ProjectDetailScreen(
                                db: widget.db,
                                sync: widget.sync,
                                projectId: p['id'] as String,
                                projectTitle: p['title'] as String? ?? '',
                              ),
                            ),
                          );
                          await _refresh();
                        },
                      );
                    },
                  ),
                ),
    );
  }
}

// ---------------------------------------------------------------------------
// Project detail – shows experiments within a project
// ---------------------------------------------------------------------------

class _ProjectDetailScreen extends StatefulWidget {
  const _ProjectDetailScreen({
    required this.db,
    required this.sync,
    required this.projectId,
    required this.projectTitle,
  });

  final LocalDatabase db;
  final SyncService sync;
  final String projectId;
  final String projectTitle;

  @override
  State<_ProjectDetailScreen> createState() => _ProjectDetailScreenState();
}

class _ProjectDetailScreenState extends State<_ProjectDetailScreen> {
  List<Map<String, dynamic>> _experiments = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      final exps = await widget.sync.api.listProjectExperiments(widget.projectId);
      if (!mounted) return;
      setState(() {
        _experiments = exps;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Failed to load experiments: $e')),
      );
    }
  }

  Future<void> _createExperiment() async {
    final titleCtl = TextEditingController();
    final bodyCtl = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Create Experiment'),
        content: SizedBox(
          width: 480,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: titleCtl,
                decoration: const InputDecoration(labelText: 'Title'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: bodyCtl,
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
            child: const Text('Create'),
          ),
        ],
      ),
    );
    if (confirmed != true || titleCtl.text.trim().isEmpty) return;

    try {
      await widget.sync.api.createExperiment(
        title: titleCtl.text.trim(),
        originalBody: bodyCtl.text.trim(),
        projectId: widget.projectId,
      );
      // Sync so local DB picks up the new experiment
      await widget.sync.syncNow();
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(e.message)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.projectTitle),
        actions: [
          IconButton(
            icon: const Icon(Icons.sync),
            onPressed: () async {
              await widget.sync.syncNow();
              await _refresh();
            },
            tooltip: 'Sync now',
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
              ? const Center(child: Text('No experiments in this project'))
              : RefreshIndicator(
                  onRefresh: _refresh,
                  child: ListView.separated(
                    itemCount: _experiments.length,
                    separatorBuilder: (_, __) => const Divider(height: 1),
                    itemBuilder: (context, index) {
                      final exp = _experiments[index];
                      final status = exp['status'] as String? ?? 'draft';
                      return ListTile(
                        leading: Icon(
                          status == 'completed' ? Icons.check_circle : Icons.science,
                          color: status == 'completed' ? Colors.green : null,
                        ),
                        title: Text(exp['title'] as String? ?? '(Untitled)'),
                        subtitle: Text(
                          '${status.toUpperCase()} | ${(exp['effectiveBody'] as String? ?? '').replaceAll('\n', ' ')}',
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        trailing: const Icon(Icons.chevron_right),
                        onTap: () async {
                          // We need the local ID for _ExperimentDetailScreen
                          // Try to find by server ID in local DB
                          final serverId = exp['id'] as String;
                          final localExps = await widget.db.listExperiments();
                          final match = localExps.where((e) => e.serverId == serverId).toList();
                          if (match.isNotEmpty) {
                            if (!context.mounted) return;
                            await Navigator.of(context).push(
                              MaterialPageRoute<void>(
                                builder: (_) => _ExperimentDetailScreen(
                                  db: widget.db,
                                  sync: widget.sync,
                                  experimentLocalId: match.first.localId,
                                ),
                              ),
                            );
                            await _refresh();
                          } else {
                            if (!context.mounted) return;
                            ScaffoldMessenger.of(context).showSnackBar(
                              const SnackBar(
                                content: Text('Experiment not yet synced locally. Try syncing first.'),
                              ),
                            );
                          }
                        },
                      );
                    },
                  ),
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
  List<Map<String, Object?>> _signatures = const [];
  List<Map<String, Object?>> _tags = const [];
  List<Map<String, dynamic>> _attachments = const [];
  List<Map<String, dynamic>> _deviations = const [];
  List<Map<String, dynamic>> _protocols = const [];
  List<Map<String, dynamic>> _dataExtracts = const [];
  List<Map<String, dynamic>> _charts = const [];

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
    setState(() => _loading = true);

    final experiments = await widget.db.listExperiments();
    if (experiments.isEmpty) {
      if (!mounted) return;
      setState(() {
        _experiment = null;
        _entries = const [];
        _comments = const [];
        _proposals = const [];
        _conflicts = const [];
        _signatures = const [];
        _tags = const [];
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

    List<Map<String, Object?>> sigs = const [];
    List<Map<String, Object?>> tags = const [];
    List<Map<String, dynamic>> attachments = const [];
    List<Map<String, dynamic>> deviations = const [];
    List<Map<String, dynamic>> protocols = const [];
    List<Map<String, dynamic>> extracts = const [];
    List<Map<String, dynamic>> charts = const [];
    if (experiment.serverId != null && experiment.serverId!.isNotEmpty) {
      sigs = await widget.db.listLocalSignatures(experiment.serverId!);
      tags = await widget.db.listLocalTags(experiment.serverId!);
      try {
        attachments = await widget.sync.api.listExperimentAttachments(experiment.serverId!);
      } catch (_) {}
      try {
        deviations = await widget.sync.api.listDeviations(experiment.serverId!);
      } catch (_) {}
      try {
        protocols = await widget.sync.api.listProtocols();
      } catch (_) {}
      try {
        extracts = await widget.sync.api.listDataExtracts(experiment.serverId!);
      } catch (_) {}
      try {
        charts = await widget.sync.api.listCharts(experiment.serverId!);
      } catch (_) {}
    }

    if (!mounted) return;
    setState(() {
      _experiment = experiment;
      _entries = entries;
      _comments = comments;
      _proposals = proposals;
      _conflicts = conflicts;
      _signatures = sigs;
      _tags = tags;
      _attachments = attachments;
      _deviations = deviations;
      _protocols = protocols;
      _dataExtracts = extracts;
      _charts = charts;
      _loading = false;
    });
  }

  Future<void> _queueAddendum() async {
    final experiment = _experiment;
    if (experiment == null || _addendumController.text.trim().isEmpty) return;
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
    if (experiment == null || _commentController.text.trim().isEmpty) return;
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
    if (experiment == null ||
        _proposalTitleController.text.trim().isEmpty ||
        _proposalBodyController.text.trim().isEmpty) return;
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

  Future<void> _addTag() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;

    final tagCtl = TextEditingController();
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Add Tag'),
        content: TextField(controller: tagCtl, decoration: const InputDecoration(labelText: 'Tag name')),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Add')),
        ],
      ),
    );
    if (confirmed != true || tagCtl.text.trim().isEmpty) return;

    try {
      await widget.sync.api.addTag(experimentId: experiment!.serverId!, tag: tagCtl.text.trim());
      await widget.sync.syncNow();
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _signExperiment() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;

    final passwordCtl = TextEditingController();
    final meaningCtl = TextEditingController(text: 'authored');
    String role = 'author';

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: const Text('Sign Experiment'),
          content: SizedBox(
            width: 400,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Re-enter your password to sign:'),
                const SizedBox(height: 12),
                TextField(
                  controller: passwordCtl,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: 'Password'),
                ),
                const SizedBox(height: 12),
                DropdownButton<String>(
                  value: role,
                  isExpanded: true,
                  items: const [
                    DropdownMenuItem(value: 'author', child: Text('Author')),
                    DropdownMenuItem(value: 'reviewer', child: Text('Reviewer')),
                    DropdownMenuItem(value: 'witness', child: Text('Witness')),
                    DropdownMenuItem(value: 'approver', child: Text('Approver')),
                  ],
                  onChanged: (v) {
                    if (v != null) setDialogState(() => role = v);
                  },
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: meaningCtl,
                  decoration: const InputDecoration(labelText: 'Meaning (e.g. authored, reviewed, approved)'),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Sign')),
          ],
        ),
      ),
    );
    if (confirmed != true || passwordCtl.text.isEmpty) return;

    try {
      await widget.sync.api.signExperiment(
        experimentId: experiment!.serverId!,
        meaning: meaningCtl.text.trim(),
        password: passwordCtl.text,
      );
      await widget.sync.syncNow();
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Experiment signed')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _markComplete() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;
    try {
      await widget.sync.api.markCompleted(experiment!.serverId!);
      await widget.sync.syncNow();
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Experiment marked complete')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _cloneExperiment() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;
    final titleCtl = TextEditingController(text: '${experiment!.title} (Clone)');
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Clone Experiment'),
        content: TextField(controller: titleCtl, decoration: const InputDecoration(labelText: 'New title')),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Clone')),
        ],
      ),
    );
    if (confirmed != true || titleCtl.text.trim().isEmpty) return;
    try {
      await widget.sync.api.cloneExperiment(sourceExperimentId: experiment.serverId!, newTitle: titleCtl.text.trim());
      await widget.sync.syncNow();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Experiment cloned')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _exportPdf() async {
    final experiment = _experiment;
    if (experiment == null) return;
    final doc = pw.Document();
    doc.addPage(
      pw.MultiPage(
        build: (ctx) => [
          pw.Header(level: 0, child: pw.Text(experiment.title)),
          pw.Text('Status: ${experiment.status.toUpperCase()}'),
          pw.SizedBox(height: 8),
          pw.Text('Effective Content', style: pw.TextStyle(fontWeight: pw.FontWeight.bold)),
          pw.Text(experiment.effectiveBody),
          pw.SizedBox(height: 12),
          pw.Text('Immutable History', style: pw.TextStyle(fontWeight: pw.FontWeight.bold)),
          ..._entries.map((e) => pw.Padding(
                padding: const pw.EdgeInsets.only(bottom: 8),
                child: pw.Column(
                  crossAxisAlignment: pw.CrossAxisAlignment.start,
                  children: [
                    pw.Text('${e.entryType.toUpperCase()} @ ${e.createdAt.toIso8601String()}', style: pw.TextStyle(fontWeight: pw.FontWeight.bold)),
                    pw.Text(e.body),
                  ],
                ),
              )),
        ],
      ),
    );
    await Printing.layoutPdf(onLayout: (_) => doc.save());
  }

  Future<void> _uploadAttachment() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;
    final file = await FilePicker.platform.pickFiles(withData: true, allowMultiple: false);
    if (file == null || file.files.isEmpty) return;
    final picked = file.files.first;
    final bytes = picked.bytes;
    if (bytes == null || bytes.isEmpty) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Could not read selected file')));
      return;
    }

    final objectKey = '${experiment!.serverId}/${DateTime.now().millisecondsSinceEpoch}_${picked.name}';
    final mimeType = picked.extension?.toLowerCase() == 'csv' ? 'text/csv' : 'application/octet-stream';
    try {
      final initiated = await widget.sync.api.initiateAttachment(
        experimentId: experiment.serverId!,
        objectKey: objectKey,
        sizeBytes: bytes.length,
        mimeType: mimeType,
      );

      final uploadUrl = initiated['uploadUrl'] as String;
      final attachmentId = initiated['attachmentId'] as String;
      final putResp = await http.put(Uri.parse(uploadUrl), headers: {'Content-Type': mimeType}, body: bytes);
      if (putResp.statusCode < 200 || putResp.statusCode >= 300) {
        throw ApiException(putResp.statusCode, 'upload failed');
      }

      final checksum = sha256.convert(bytes).toString();
      await widget.sync.api.completeAttachment(
        attachmentId: attachmentId,
        checksum: checksum,
        sizeBytes: bytes.length,
      );

      await widget.sync.syncNow();
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Attachment uploaded')));
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Upload failed: $e')));
    }
  }

  Future<void> _downloadAttachment(Map<String, dynamic> attachment) async {
    final attachmentId = attachment['id'] as String?;
    if (attachmentId == null || attachmentId.isEmpty) return;
    try {
      final resp = await widget.sync.api.downloadAttachment(attachmentId);
      final url = resp['downloadUrl'] as String? ?? '';
      if (url.isEmpty) return;
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('Download URL'),
          content: SelectableText(url),
          actions: [
            TextButton(
              onPressed: () async {
                await Clipboard.setData(ClipboardData(text: url));
                if (context.mounted) {
                  Navigator.pop(context);
                  ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('URL copied')));
                }
              },
              child: const Text('Copy URL'),
            ),
            FilledButton(onPressed: () => Navigator.pop(context), child: const Text('Close')),
          ],
        ),
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _linkProtocol() async {
    final experiment = _experiment;
    if (experiment?.serverId == null || _protocols.isEmpty) return;
    String? protocolId = _protocols.first['protocolId'] as String?;
    final versionCtl = TextEditingController(text: '1');
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: const Text('Link Protocol'),
          content: SizedBox(
            width: 420,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                DropdownButtonFormField<String>(
                  value: protocolId,
                  items: _protocols
                      .map((p) => DropdownMenuItem<String>(
                            value: p['protocolId'] as String?,
                            child: Text((p['title'] as String?) ?? ''),
                          ))
                      .toList(),
                  onChanged: (v) => setDialogState(() => protocolId = v),
                  decoration: const InputDecoration(labelText: 'Protocol'),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: versionCtl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(labelText: 'Version Number'),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Link')),
          ],
        ),
      ),
    );
    if (confirmed != true || protocolId == null) return;
    final versionNum = int.tryParse(versionCtl.text.trim()) ?? 1;
    try {
      await widget.sync.api.linkProtocol(
        experimentId: experiment!.serverId!,
        protocolId: protocolId!,
        versionNum: versionNum,
      );
      await widget.sync.syncNow();
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _recordDeviation() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;
    final protocolCtl = TextEditingController();
    final descCtl = TextEditingController();
    String severity = 'minor';

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: const Text('Record Deviation'),
          content: SizedBox(
            width: 480,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: protocolCtl,
                  decoration: const InputDecoration(labelText: 'Protocol ID'),
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<String>(
                  value: severity,
                  items: const [
                    DropdownMenuItem(value: 'minor', child: Text('Minor')),
                    DropdownMenuItem(value: 'major', child: Text('Major')),
                    DropdownMenuItem(value: 'critical', child: Text('Critical')),
                  ],
                  onChanged: (v) => setDialogState(() => severity = v ?? 'minor'),
                  decoration: const InputDecoration(labelText: 'Severity'),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: descCtl,
                  maxLines: 4,
                  decoration: const InputDecoration(labelText: 'Deviation Description'),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Record')),
          ],
        ),
      ),
    );
    if (confirmed != true || protocolCtl.text.trim().isEmpty || descCtl.text.trim().isEmpty) return;
    try {
      await widget.sync.api.recordDeviation(
        experimentId: experiment!.serverId!,
        protocolId: protocolCtl.text.trim(),
        description: descCtl.text.trim(),
        severity: severity,
      );
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _parseCsvDataExtract() async {
    final experiment = _experiment;
    if (experiment?.serverId == null) return;
    final file = await FilePicker.platform.pickFiles(withData: true, allowMultiple: false, type: FileType.custom, allowedExtensions: ['csv']);
    if (file == null || file.files.isEmpty || file.files.first.bytes == null) return;
    final bytes = file.files.first.bytes!;
    final csvData = utf8.decode(bytes, allowMalformed: true);
    try {
      await widget.sync.api.parseCSV(
        attachmentId: '',
        experimentId: experiment!.serverId!,
        csvData: csvData,
      );
      await _refresh();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('CSV parsed into data extract')));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _createChartConfig() async {
    final experiment = _experiment;
    if (experiment?.serverId == null || _dataExtracts.isEmpty) return;
    String extractId = (_dataExtracts.first['dataExtractId'] ?? '').toString();
    String chartType = 'line';
    final titleCtl = TextEditingController();
    final xCtl = TextEditingController();
    final yCtl = TextEditingController();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (ctx, setDialogState) => AlertDialog(
          title: const Text('Create Chart'),
          content: SizedBox(
            width: 520,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                DropdownButtonFormField<String>(
                  value: extractId,
                  decoration: const InputDecoration(labelText: 'Data Extract'),
                  items: _dataExtracts
                      .map((de) => DropdownMenuItem<String>(
                            value: (de['dataExtractId'] ?? '').toString(),
                            child: Text('Extract ${(de['dataExtractId'] ?? '').toString().substring(0, 8)} • rows ${de['rowCount'] ?? 0}'),
                          ))
                      .toList(),
                  onChanged: (v) => setDialogState(() => extractId = v ?? extractId),
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<String>(
                  value: chartType,
                  decoration: const InputDecoration(labelText: 'Chart Type'),
                  items: const [
                    DropdownMenuItem(value: 'line', child: Text('Line')),
                    DropdownMenuItem(value: 'bar', child: Text('Bar')),
                    DropdownMenuItem(value: 'scatter', child: Text('Scatter')),
                  ],
                  onChanged: (v) => setDialogState(() => chartType = v ?? 'line'),
                ),
                const SizedBox(height: 12),
                TextField(controller: titleCtl, decoration: const InputDecoration(labelText: 'Title')),
                const SizedBox(height: 12),
                TextField(controller: xCtl, decoration: const InputDecoration(labelText: 'X column header')),
                const SizedBox(height: 12),
                TextField(controller: yCtl, decoration: const InputDecoration(labelText: 'Y column header (single)')),
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

    if (confirmed != true || xCtl.text.trim().isEmpty || yCtl.text.trim().isEmpty) return;

    try {
      await widget.sync.api.createChart(
        experimentId: experiment!.serverId!,
        dataExtractId: extractId,
        chartType: chartType,
        title: titleCtl.text.trim().isEmpty ? 'Chart' : titleCtl.text.trim(),
        xColumn: xCtl.text.trim(),
        yColumns: [yCtl.text.trim()],
      );
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _showEntryDiff(int index) async {
    if (index <= 0 || index >= _entries.length) return;
    final prev = _entries[index - 1];
    final curr = _entries[index];
    await showDialog<void>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Entry Diff (Previous vs Current)'),
        content: SizedBox(
          width: 900,
          child: Row(
            children: [
              Expanded(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text('Previous', style: TextStyle(fontWeight: FontWeight.bold)),
                    const SizedBox(height: 8),
                    Expanded(child: SingleChildScrollView(child: SelectableText(prev.body))),
                  ],
                ),
              ),
              const VerticalDivider(width: 20),
              Expanded(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text('Current', style: TextStyle(fontWeight: FontWeight.bold)),
                    const SizedBox(height: 8),
                    Expanded(child: SingleChildScrollView(child: SelectableText(curr.body))),
                  ],
                ),
              ),
            ],
          ),
        ),
        actions: [
          FilledButton(onPressed: () => Navigator.pop(context), child: const Text('Close')),
        ],
      ),
    );
  }

  Widget _buildChartPreviewCard(Map<String, dynamic> chart) {
    final dataExtractId = (chart['dataExtractId'] ?? '').toString();
    final xColumn = (chart['xColumn'] ?? '').toString();
    final yColumnsRaw = chart['yColumns'];
    final yColumns = yColumnsRaw is List ? yColumnsRaw.map((e) => e.toString()).toList() : <String>[];
    final yColumn = yColumns.isNotEmpty ? yColumns.first : '';
    final chartType = (chart['chartType'] ?? 'line').toString();

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text((chart['title'] ?? 'Chart').toString(), style: const TextStyle(fontWeight: FontWeight.bold)),
            const SizedBox(height: 8),
            SizedBox(
              height: 220,
              child: FutureBuilder<Map<String, dynamic>>(
                future: widget.sync.api.getDataExtract(dataExtractId),
                builder: (context, snapshot) {
                  if (!snapshot.hasData) {
                    return const Center(child: CircularProgressIndicator());
                  }
                  final extract = snapshot.data!;
                  final headers = (extract['columnHeaders'] as List?)?.map((e) => e.toString()).toList() ?? const <String>[];
                  final rows = (extract['sampleRows'] as List?)?.cast<List<dynamic>>() ?? const <List<dynamic>>[];
                  final xIdx = headers.indexOf(xColumn);
                  final yIdx = headers.indexOf(yColumn);
                  if (xIdx < 0 || yIdx < 0) {
                    return const Center(child: Text('Chart columns not found in extract'));
                  }

                  final spots = <FlSpot>[];
                  for (final row in rows) {
                    if (row.length <= yIdx) continue;
                    final xv = double.tryParse(row[xIdx].toString());
                    final yv = double.tryParse(row[yIdx].toString());
                    if (xv != null && yv != null) {
                      spots.add(FlSpot(xv, yv));
                    }
                  }
                  if (spots.isEmpty) {
                    return const Center(child: Text('No numeric sample data to plot'));
                  }

                  if (chartType == 'bar') {
                    final bars = spots.asMap().entries.map((e) {
                      return BarChartGroupData(x: e.key, barRods: [BarChartRodData(toY: e.value.y)]);
                    }).toList();
                    return BarChart(BarChartData(barGroups: bars));
                  }

                  final isScatter = chartType == 'scatter';
                  return LineChart(
                    LineChartData(
                      lineBarsData: [
                        LineChartBarData(
                          spots: spots,
                          isCurved: chartType == 'line',
                          barWidth: isScatter ? 0 : 2,
                          dotData: FlDotData(show: true),
                        ),
                      ],
                    ),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (_experiment == null) {
      return const Scaffold(body: Center(child: Text('Experiment not available')));
    }

    final experiment = _experiment!;

    return Scaffold(
      appBar: AppBar(
        title: Text(experiment.title),
        actions: [
          IconButton(
            icon: const Icon(Icons.picture_as_pdf),
            onPressed: _exportPdf,
            tooltip: 'Export / Print PDF',
          ),
          IconButton(
            icon: const Icon(Icons.copy_all),
            onPressed: _cloneExperiment,
            tooltip: 'Clone experiment',
          ),
          if (experiment.status != 'completed')
            IconButton(
              icon: const Icon(Icons.task_alt),
              onPressed: _markComplete,
              tooltip: 'Mark complete',
            ),
          IconButton(
            icon: const Icon(Icons.verified),
            onPressed: _signExperiment,
            tooltip: 'Sign experiment',
          ),
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
          // Status & content card
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Status: ${experiment.status}'),
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 8,
                    children: [
                      OutlinedButton.icon(
                        onPressed: _linkProtocol,
                        icon: const Icon(Icons.link),
                        label: const Text('Link Protocol'),
                      ),
                      OutlinedButton.icon(
                        onPressed: _recordDeviation,
                        icon: const Icon(Icons.rule_folder),
                        label: const Text('Record Deviation'),
                      ),
                      OutlinedButton.icon(
                        onPressed: _uploadAttachment,
                        icon: const Icon(Icons.upload_file),
                        label: const Text('Upload Attachment'),
                      ),
                      OutlinedButton.icon(
                        onPressed: _parseCsvDataExtract,
                        icon: const Icon(Icons.table_chart),
                        label: const Text('Parse CSV'),
                      ),
                      OutlinedButton.icon(
                        onPressed: _createChartConfig,
                        icon: const Icon(Icons.auto_graph),
                        label: const Text('Create Chart'),
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  const Text('Effective content'),
                  const SizedBox(height: 4),
                  Text(experiment.effectiveBody),
                ],
              ),
            ),
          ),

          // Tags
          const SizedBox(height: 12),
          Row(
            children: [
              const Text('Tags', style: TextStyle(fontWeight: FontWeight.bold)),
              const SizedBox(width: 8),
              ActionChip(
                avatar: const Icon(Icons.add, size: 16),
                label: const Text('Add'),
                onPressed: _addTag,
              ),
            ],
          ),
          const SizedBox(height: 6),
          Wrap(
            spacing: 6,
            runSpacing: 4,
            children: _tags.map((t) => Chip(label: Text(t['name'] as String? ?? ''))).toList(),
          ),

          // Signatures
          const SizedBox(height: 12),
          const Text('Signatures', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          if (_signatures.isEmpty)
            const Text('No signatures yet')
          else
            ..._signatures.map((s) {
              final role = s['role'] as String? ?? '';
              final email = s['signer_email'] as String? ?? s['signer_user_id'] as String? ?? '';
              final signedAt = s['signed_at'] as int?;
              final dt = signedAt != null ? DateTime.fromMillisecondsSinceEpoch(signedAt) : null;
              return ListTile(
                dense: true,
                leading: const Icon(Icons.verified, color: Colors.green, size: 20),
                title: Text('$email ($role)'),
                subtitle: dt != null ? Text('Signed: ${dt.toIso8601String()}') : null,
              );
            }),

          // Immutable history
          const SizedBox(height: 12),
          const Text('Immutable history', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          ..._entries.asMap().entries.map(
            (kv) {
              final index = kv.key;
              final entry = kv.value;
              return Card(
              child: ListTile(
                title: Text(entry.entryType.toUpperCase()),
                subtitle: Text(entry.body),
                trailing: Wrap(
                  spacing: 8,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    Text('${entry.createdAt.hour}:${entry.createdAt.minute.toString().padLeft(2, '0')}'),
                    if (index > 0)
                      IconButton(
                        icon: const Icon(Icons.compare_arrows),
                        tooltip: 'Diff with previous entry',
                        onPressed: () => _showEntryDiff(index),
                      ),
                  ],
                ),
              ),
            );
            },
          ),

          // Attachments
          const SizedBox(height: 12),
          const Text('Attachments', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          if (_attachments.isEmpty)
            const Text('No attachments yet')
          else
            ..._attachments.map(
              (att) => Card(
                child: ListTile(
                  leading: const Icon(Icons.attach_file),
                  title: Text((att['objectKey'] ?? '').toString()),
                  subtitle: Text('Status: ${(att['status'] ?? '').toString()} • ${(att['sizeBytes'] ?? 0)} bytes'),
                  trailing: IconButton(
                    icon: const Icon(Icons.download),
                    onPressed: () => _downloadAttachment(att),
                  ),
                ),
              ),
            ),

          // Protocol deviations
          const SizedBox(height: 12),
          const Text('Protocol Deviations', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          if (_deviations.isEmpty)
            const Text('No deviations recorded')
          else
            ..._deviations.map(
              (d) => Card(
                child: ListTile(
                  title: Text((d['deviationType'] ?? d['severity'] ?? 'deviation').toString()),
                  subtitle: Text((d['rationale'] ?? d['description'] ?? '').toString()),
                ),
              ),
            ),

          // Data Visualization
          const SizedBox(height: 12),
          const Text('Data Visualizations', style: TextStyle(fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          if (_charts.isEmpty)
            const Text('No charts configured yet')
          else
            ..._charts.map(_buildChartPreviewCard),

          // Add addendum
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
          FilledButton(onPressed: _queueAddendum, child: const Text('Queue Addendum')),

          // Comments
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
          OutlinedButton(onPressed: _queueComment, child: const Text('Queue Comment')),

          // Proposals
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
          OutlinedButton(onPressed: _queueProposal, child: const Text('Queue Proposal')),

          // Conflicts
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
