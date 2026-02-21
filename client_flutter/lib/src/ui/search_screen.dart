import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/sync_service.dart';

class SearchScreen extends StatefulWidget {
  const SearchScreen({super.key, required this.sync});
  final SyncService sync;

  @override
  State<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends State<SearchScreen> {
  final _queryController = TextEditingController();
  List<dynamic> _results = const [];
  bool _searching = false;
  bool _hasSearched = false;

  @override
  void dispose() {
    _queryController.dispose();
    super.dispose();
  }

  Future<void> _doSearch() async {
    final q = _queryController.text.trim();
    if (q.isEmpty) return;

    setState(() {
      _searching = true;
      _hasSearched = true;
    });

    try {
      final response = await widget.sync.api.search(query: q);
      final results = (response['results'] as List<dynamic>?) ?? [];
      if (!mounted) return;
      setState(() {
        _results = results;
        _searching = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _searching = false);
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _queryController,
                  decoration: const InputDecoration(
                    hintText: 'Search experiments and protocols...',
                    prefixIcon: Icon(Icons.search),
                    border: OutlineInputBorder(),
                    isDense: true,
                  ),
                  onSubmitted: (_) => _doSearch(),
                ),
              ),
              const SizedBox(width: 8),
              FilledButton(
                onPressed: _searching ? null : _doSearch,
                child: const Text('Search'),
              ),
            ],
          ),
        ),
        Expanded(
          child: _searching
              ? const Center(child: CircularProgressIndicator())
              : !_hasSearched
                  ? const Center(child: Text('Enter a query to search'))
                  : _results.isEmpty
                      ? const Center(child: Text('No results found'))
                      : ListView.separated(
                          itemCount: _results.length,
                          separatorBuilder: (_, __) => const Divider(height: 1),
                          itemBuilder: (context, i) {
                            final r = _results[i] as Map<String, dynamic>;
                            final kind = r['kind'] as String? ?? 'experiment';
                            final headline = r['headline'] as String? ?? '';
                            return ListTile(
                              leading: Icon(
                                kind == 'protocol' ? Icons.article : Icons.science,
                                color: kind == 'protocol' ? Colors.blue : Colors.teal,
                              ),
                              title: Text(r['title'] as String? ?? '(untitled)'),
                              subtitle: Text(
                                headline.isNotEmpty ? headline : kind.toUpperCase(),
                                maxLines: 2,
                                overflow: TextOverflow.ellipsis,
                              ),
                            );
                          },
                        ),
        ),
      ],
    );
  }
}
