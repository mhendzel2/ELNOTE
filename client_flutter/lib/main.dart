import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:sqflite_common_ffi_web/sqflite_ffi_web.dart';

import 'src/data/local_database.dart';
import 'src/ui/app.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  LocalDatabase? db;
  Object? startupError;
  StackTrace? startupStack;

  try {
    if (kIsWeb) {
      // sqflite global API must point to a web-capable factory before opening DB.
      databaseFactory = databaseFactoryFfiWeb;
    } else if (Platform.isWindows || Platform.isLinux || Platform.isMacOS) {
      // Initialize sqflite FFI for desktop platforms.
      sqfliteFfiInit();
      databaseFactory = databaseFactoryFfi;
    }

    db = LocalDatabase();
    await db.open();
  } catch (e, st) {
    startupError = e;
    startupStack = st;
  }

  runApp(
    startupError == null && db != null
        ? ElnoteApplication(db: db)
        : _StartupErrorApp(
            error: startupError ?? StateError('unknown startup error'),
            stackTrace: startupStack,
          ),
  );
}

class _StartupErrorApp extends StatelessWidget {
  const _StartupErrorApp({
    required this.error,
    required this.stackTrace,
  });

  final Object error;
  final StackTrace? stackTrace;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      home: Scaffold(
        appBar: AppBar(title: const Text('ELNOTE Startup Error')),
        body: Padding(
          padding: const EdgeInsets.all(16),
          child: SingleChildScrollView(
            child: SelectableText(
              'Startup failed:\n\n$error\n\n'
              'Stack trace:\n${stackTrace ?? '(none)'}',
            ),
          ),
        ),
      ),
    );
  }
}
