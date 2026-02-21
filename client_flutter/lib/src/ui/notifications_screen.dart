import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/local_database.dart';
import '../data/sync_service.dart';

class NotificationsScreen extends StatefulWidget {
  const NotificationsScreen({super.key, required this.db, required this.sync});
  final LocalDatabase db;
  final SyncService sync;

  @override
  State<NotificationsScreen> createState() => _NotificationsScreenState();
}

class _NotificationsScreenState extends State<NotificationsScreen> {
  List<Map<String, Object?>> _notifications = const [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    try {
      await widget.sync.refreshNotifications();
    } catch (_) {}
    final rows = await widget.db.listLocalNotifications();
    if (!mounted) return;
    setState(() {
      _notifications = rows;
      _loading = false;
    });
  }

  Future<void> _markAllRead() async {
    try {
      await widget.sync.api.markAllNotificationsRead();
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _markRead(String notificationId) async {
    try {
      await widget.sync.api.markNotificationRead(notificationId);
      await widget.db.markNotificationReadLocal(notificationId);
      await _refresh();
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const Center(child: CircularProgressIndicator());

    return Column(
      children: [
        if (_notifications.isNotEmpty)
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                TextButton.icon(
                  onPressed: _markAllRead,
                  icon: const Icon(Icons.done_all, size: 18),
                  label: const Text('Mark all read'),
                ),
              ],
            ),
          ),
        Expanded(
          child: _notifications.isEmpty
              ? const Center(child: Text('No notifications'))
              : ListView.separated(
                  itemCount: _notifications.length,
                  separatorBuilder: (_, __) => const Divider(height: 1),
                  itemBuilder: (context, i) {
                    final n = _notifications[i];
                    final isRead = (n['is_read'] as int? ?? 0) == 1;
                    final eventType = n['event_type'] as String? ?? '';
                    return ListTile(
                      leading: Icon(
                        _iconForEventType(eventType),
                        color: isRead ? Colors.grey : Theme.of(context).colorScheme.primary,
                      ),
                      title: Text(
                        n['title'] as String? ?? '',
                        style: TextStyle(fontWeight: isRead ? FontWeight.normal : FontWeight.bold),
                      ),
                      subtitle: Text(
                        n['body'] as String? ?? '',
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                      trailing: isRead
                          ? null
                          : IconButton(
                              icon: const Icon(Icons.check, size: 18),
                              onPressed: () => _markRead(n['notification_id'] as String),
                              tooltip: 'Mark read',
                            ),
                    );
                  },
                ),
        ),
      ],
    );
  }

  IconData _iconForEventType(String eventType) {
    if (eventType.contains('sign')) return Icons.verified;
    if (eventType.contains('comment')) return Icons.comment;
    if (eventType.contains('protocol')) return Icons.article;
    if (eventType.contains('deviation')) return Icons.warning;
    return Icons.notifications;
  }
}
