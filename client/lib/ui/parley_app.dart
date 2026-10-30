import 'dart:async';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
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
    this.promptForToken = kIsWeb,
    this.firstRun,
    this.showOnboarding = false,
  });

  final ListenSession Function() createSession;

  /// Native builds edit and persist Provider settings; web is read-only.
  final SettingsController? settings;
  final bool readOnlySettings;

  /// Web asks for the access token after a failed connect and retries.
  final bool promptForToken;

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
  bool _promptingToken = false;
  late bool _onboarded = !widget.showOnboarding;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _session = widget.createSession();
    _session.addListener(_maybePromptToken);
    _session.connect();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _session.removeListener(_maybePromptToken);
    _session.dispose();
    super.dispose();
  }

  /// Web has no settings field for the access token, so a rejected handshake
  /// asks for it here and retries. The token stays in memory for this tab.
  Future<void> _maybePromptToken() async {
    if (!widget.promptForToken ||
        _promptingToken ||
        _session.status != SessionStatus.failed ||
        !_session.canRetryWithToken) {
      return;
    }
    final context = _navigatorKey.currentContext;
    if (context == null) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _maybePromptToken());
      return;
    }
    _promptingToken = true;
    final token = await showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (_) => const AccessTokenDialog(),
    );
    _promptingToken = false;
    if (!mounted || token == null) return;
    await _session.retryWithToken(token);
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

/// The web client's only credential field; it exists after a failed connect,
/// never in Settings.
class AccessTokenDialog extends StatefulWidget {
  const AccessTokenDialog({super.key});

  @override
  State<AccessTokenDialog> createState() => _AccessTokenDialogState();
}

class _AccessTokenDialogState extends State<AccessTokenDialog> {
  final _token = TextEditingController();

  @override
  void dispose() {
    _token.dispose();
    super.dispose();
  }

  void _submit() => Navigator.of(context).pop(_token.text);

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(l10n.accessTokenTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(l10n.accessTokenPrompt),
          const SizedBox(height: 12),
          TextField(
            key: const Key('access-token'),
            controller: _token,
            autofocus: true,
            obscureText: true,
            autocorrect: false,
            enableSuggestions: false,
            onSubmitted: (_) => _submit(),
            decoration: const InputDecoration(border: OutlineInputBorder()),
          ),
        ],
      ),
      actions: [
        TextButton(
          key: const Key('access-token-cancel'),
          onPressed: () => Navigator.of(context).pop(),
          child: Text(l10n.accessTokenCancel),
        ),
        FilledButton(
          key: const Key('access-token-connect'),
          onPressed: _submit,
          child: Text(l10n.accessTokenConnect),
        ),
      ],
    );
  }
}
