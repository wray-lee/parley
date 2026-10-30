import 'dart:async';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:material_ui/material_ui.dart';

import '../l10n/parley_localizations.dart';
import '../model/mode.dart';
import '../session/listen_session.dart';
import '../settings/settings_controller.dart';
import '../theme.dart';
import 'converse_screen.dart';
import 'listen_screen.dart';
import 'settings_screen.dart';
import 'start_screen.dart';

class ParleyApp extends StatefulWidget {
  const ParleyApp({
    super.key,
    required this.createSession,
    this.settings,
    this.readOnlySettings = kIsWeb,
  });

  final ListenSession Function() createSession;

  /// Native builds edit and persist Provider settings; web is read-only.
  final SettingsController? settings;
  final bool readOnlySettings;

  @override
  State<ParleyApp> createState() => _ParleyAppState();
}

class _ParleyAppState extends State<ParleyApp> {
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();
  late final ListenSession _session;
  bool _started = false;

  @override
  void initState() {
    super.initState();
    _session = widget.createSession();
    _session.connect();
  }

  @override
  void dispose() {
    _session.dispose();
    super.dispose();
  }

  void _start() {
    setState(() => _started = true);
    unawaited(_session.start());
  }

  void _openSettings() {
    _navigatorKey.currentState?.push(
      MaterialPageRoute<void>(
        builder: (_) => SettingsScreen(
          session: _session,
          controller: widget.settings,
          readOnly: widget.readOnlySettings,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    // The read-only web summary stays reachable from the header, but the
    // Demo banner must not promise a key field the read-only screen lacks.
    final onUseKeys = widget.readOnlySettings ? null : _openSettings;
    return MaterialApp(
      title: 'Parley',
      navigatorKey: _navigatorKey,
      debugShowCheckedModeBanner: false,
      theme: parleyTheme(Brightness.light),
      darkTheme: parleyTheme(Brightness.dark),
      themeMode: ThemeMode.dark,
      localizationsDelegates: parleyLocalizationDelegates,
      supportedLocales: parleySupportedLocales,
      home: !_started
          ? StartScreen(
              session: _session,
              onStart: _start,
              onSettings: _openSettings,
              onUseKeys: onUseKeys,
            )
          : _session.selectedMode == SessionMode.converse
          ? ConverseScreen(
              session: _session,
              onSettings: _openSettings,
              onUseKeys: onUseKeys,
            )
          : ListenScreen(
              session: _session,
              onSettings: _openSettings,
              onUseKeys: onUseKeys,
            ),
    );
  }
}
