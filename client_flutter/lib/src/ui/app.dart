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
          ..._entries.map(
            (entry) => Card(
              child: ListTile(
                title: Text(entry.entryType.toUpperCase()),
                subtitle: Text(entry.body),
                trailing: Text('${entry.createdAt.hour}:${entry.createdAt.minute.toString().padLeft(2, '0')}'),
              ),
            ),
          ),

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
