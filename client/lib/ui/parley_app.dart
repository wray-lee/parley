import 'dart:async';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:material_ui/material_ui.dart';

import '../l10n/parley_localizations.dart';
import '../model/mode.dart';
import '../platform/first_run_store_api.dart';
import '../session/listen_session.dart';
import '../settings/settings_controller.dart';
import '../theme.dart';
import 'converse_screen.dart';
import 'listen_screen.dart';
import 'onboarding_screen.dart';
import 'settings_screen.dart';
import 'start_screen.dart';

class ParleyApp extends StatefulWidget {
  const ParleyApp({
    super.key,
    required this.createSession,
    this.settings,
    this.readOnlySettings = kIsWeb,
    this.firstRun,
    this.showOnboarding = false,
  });

  final ListenSession Function() createSession;

  /// Native builds edit and persist Provider settings; web is read-only.
  final SettingsController? settings;
  final bool readOnlySettings;

  /// First-run flags; null means the host has nowhere to keep them (tests).
  final FirstRunStore? firstRun;

  /// A fresh install starts on the three-step onboarding, not on Start.
  final bool showOnboarding;

  @override
  State<ParleyApp> createState() => _ParleyAppState();
}

class _ParleyAppState extends State<ParleyApp> with WidgetsBindingObserver {
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();
  late final ListenSession _session;
  bool _started = false;
  late bool _onboarded = !widget.showOnboarding;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _session = widget.createSession();
    _session.connect();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _session.dispose();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // Foreground-only (S2): leaving the foreground ends the Session so the
    // microphone and TTS stop at once; the host is stopped by HostLifecycle and
    // its next hello brings this Session back to ready.
    if (state == AppLifecycleState.paused) unawaited(_session.stop());
  }

  void _start() {
    setState(() => _started = true);
    unawaited(_session.start());
  }

  /// The third onboarding step either starts the keyless demo right away or
  /// lands on Start when the user still has keys or a server to set up.
  void _finishOnboarding({required bool startNow}) {
    final store = widget.firstRun;
    if (store != null) unawaited(store.markSeen('onboarding'));
    setState(() {
      _onboarded = true;
      if (startNow) _started = true;
    });
    if (startNow) unawaited(_session.start());
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
      home: !_onboarded
          ? OnboardingScreen(
              session: _session,
              onFinish: _finishOnboarding,
              onSettings: _openSettings,
              onUseKeys: onUseKeys,
            )
          : !_started
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
              firstRun: widget.firstRun,
            )
          : ListenScreen(
              session: _session,
              onSettings: _openSettings,
              onUseKeys: onUseKeys,
            ),
    );
  }
}
