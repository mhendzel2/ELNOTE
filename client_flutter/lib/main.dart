import 'package:flutter/material.dart';

import 'src/data/local_database.dart';
import 'src/ui/app.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  final db = LocalDatabase();
  await db.open();

  runApp(ElnoteApplication(db: db));
}
