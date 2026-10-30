import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../model/protocol.dart';
import '../platform/keep_awake.dart';
import '../session/listen_session.dart';
import '../theme.dart';
import 'caption_stack.dart';

/// The Listen surface: status and pair from `hello`, the caption stack, and
/// one Start/Stop bar. Connection details live behind [ListenSession].
/// Chrome clamps at 1.3× text; captions never clamp and step through
/// 24/28/34/40 on top of the OS scale.
class ListenScreen extends StatefulWidget {
  const ListenScreen({
    super.key,
    required this.session,
    this.advanced = false,
    this.wakeLock = keepAwake,
    this.onSettings,
    this.onUseKeys,
  });

  final ListenSession session;

  /// Latency chips and merge counts live under Advanced/About.
  final bool advanced;
  final Future<void> Function(bool on) wakeLock;

  /// Opens Settings from the header; the read-only web build still shows it.
  final VoidCallback? onSettings;

  /// The Demo banner's "Use my keys" CTA; null on read-only web settings,
  /// where there is no key field to reach.
  final VoidCallback? onUseKeys;

  @override
  State<ListenScreen> createState() => _ListenScreenState();
}

class _ListenScreenState extends State<ListenScreen> {
  static const _steps = [24.0, 28.0, 34.0, 40.0];

  int _step = 1;
  bool _focus = false;
  bool _awake = false;
  late bool _advanced = widget.advanced;

  void _toggleAdvanced() => setState(() => _advanced = !_advanced);

  void _cycleCaptionSize() {
    setState(() => _step = (_step + 1) % _steps.length);
  }

  @override
  void initState() {
    super.initState();
    widget.session.addListener(_syncAwake);
    _syncAwake();
  }

  /// The screen stays awake for the whole Session (SYNTHESIS §1.4), not only
  /// in Focus mode.
  void _syncAwake() {
    unawaited(_setAwake(widget.session.status == SessionStatus.listening));
  }

  void _toggleFocus() => setState(() => _focus = !_focus);

  Future<void> _setAwake(bool on) async {
    if (_awake == on) return;
    _awake = on;
    try {
      await widget.wakeLock(on);
    } catch (error) {
      // A missing plugin must not take the session down.
      debugPrint('PARLEY wake lock $error');
    }
  }

  @override
  void dispose() {
    widget.session.removeListener(_syncAwake);
    // Guarded like every wake-lock call: teardown may outlive the plugin.
    unawaited(_setAwake(false));
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.session,
      builder: (context, _) {
        final session = widget.session;
        final colors = ParleyColors.of(context);
        final l10n = Localizations.of<AppLocalizations>(
          context,
          AppLocalizations,
        );
        final captionSize = _focus && _steps[_step] < 34 ? 34.0 : _steps[_step];
        final captions = CaptionStack(
          segments: session.segments,
          demo: session.demo,
          live: session.live,
          held: session.held,
          merges: session.merges,
          inlineError: session.inlineError,
          captionSize: captionSize,
          showSource: !_focus,
          showLatency: _advanced,
          advanced: _advanced,
          focus: _focus,
          onSkipLive: session.skipLive,
          onUseKeys: widget.onUseKeys,
        );
        final banners = [
          if (session.error != null)
            _ErrorBanner(message: session.error!, colors: colors),
          if (session.audioError != null) const _AudioUnavailableChip(),
        ];
        if (_focus) {
          return Scaffold(
            body: SafeArea(
              child: Column(
                children: [
                  for (final banner in banners)
                    MediaQuery.withClampedTextScaling(
                      maxScaleFactor: 1.3,
                      child: banner,
                    ),
                  Expanded(
                    child: Stack(
                      children: [
                        Positioned.fill(child: captions),
                        Positioned(
                          right: parleyMargin(context),
                          bottom: parleyMargin(context),
                          child: IconButton.filledTonal(
                            key: const Key('exit-focus'),
                            tooltip: l10n?.exitFocusMode ?? 'Exit focus mode',
                            onPressed: _toggleFocus,
                            constraints: const BoxConstraints(
                              minWidth: 48,
                              minHeight: 48,
                            ),
                            icon: const Icon(Icons.fullscreen_exit_rounded),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          );
        }
        return Scaffold(
          body: SafeArea(
            child: Column(
              children: [
                MediaQuery.withClampedTextScaling(
                  maxScaleFactor: 1.3,
                  child: _Header(
                    session: session,
                    colors: colors,
                    advanced: _advanced,
                    onCycleCaptionSize: _cycleCaptionSize,
                    onToggleFocus: _toggleFocus,
                    onToggleAdvanced: _toggleAdvanced,
                    onSettings: widget.onSettings,
                  ),
                ),
                for (final banner in banners)
                  MediaQuery.withClampedTextScaling(
                    maxScaleFactor: 1.3,
                    child: banner,
                  ),
                Expanded(child: captions),
                MediaQuery.withClampedTextScaling(
                  maxScaleFactor: 1.3,
                  child: _StartBar(session: session),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({
    required this.session,
    required this.colors,
    required this.advanced,
    required this.onCycleCaptionSize,
    required this.onToggleFocus,
    required this.onToggleAdvanced,
    this.onSettings,
  });

  final ListenSession session;
  final ParleyColors colors;
  final bool advanced;
  final VoidCallback onCycleCaptionSize;
  final VoidCallback onToggleFocus;
  final VoidCallback onToggleAdvanced;
  final VoidCallback? onSettings;

  @override
  Widget build(BuildContext context) {
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    final listening = session.status == SessionStatus.listening;
    return Padding(
      padding: EdgeInsets.fromLTRB(
        parleyMargin(context),
        12,
        parleyMargin(context),
        8,
      ),
      child: Column(
        children: [
          Row(
            children: [
              Text(
                'Parley',
                style: TextStyle(
                  color: colors.ink,
                  fontSize: 20,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(width: 16),
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: listening ? colors.live : colors.muted,
                ),
              ),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  _statusLabel(l10n, session.status),
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    color: listening ? colors.liveText : colors.muted,
                    fontSize: 14,
                  ),
                ),
              ),
              Tooltip(
                message: l10n?.captionSize ?? 'Caption size',
                child: TextButton(
                  key: const Key('caption-size'),
                  onPressed: onCycleCaptionSize,
                  style: TextButton.styleFrom(
                    minimumSize: const Size(48, 48),
                    foregroundColor: colors.ink,
                  ),
                  child: const Text('Aa'),
                ),
              ),
              IconButton(
                key: const Key('focus-mode'),
                tooltip: l10n?.focusMode ?? 'Focus mode',
                onPressed: onToggleFocus,
                constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
                icon: Icon(Icons.fullscreen_rounded, color: colors.ink),
              ),
              PopupMenuButton<String>(
                key: const Key('more-menu'),
                tooltip: l10n?.more ?? 'More',
                padding: const EdgeInsets.all(12),
                icon: Icon(Icons.more_vert, color: colors.ink),
                onSelected: (value) {
                  if (value == 'advanced') onToggleAdvanced();
                  if (value == 'settings') onSettings?.call();
                },
                itemBuilder: (context) => [
                  CheckedPopupMenuItem<String>(
                    value: 'advanced',
                    checked: advanced,
                    child: Text(l10n?.settingsAdvanced ?? 'Advanced'),
                  ),
                  if (onSettings != null)
                    PopupMenuItem<String>(
                      value: 'settings',
                      child: Text(l10n?.settings ?? 'Settings'),
                    ),
                ],
              ),
            ],
          ),
          if (session.pairs.isNotEmpty)
            Align(
              alignment: Alignment.centerLeft,
              child: DropdownButton<LanguagePair>(
                value: session.selectedPair,
                underline: const SizedBox.shrink(),
                dropdownColor: colors.control,
                style: TextStyle(color: colors.ink, fontSize: 14),
                items: [
                  for (final pair in session.pairs)
                    DropdownMenuItem(value: pair, child: Text(pair.label)),
                ],
                onChanged: session.selectPair,
              ),
            ),
        ],
      ),
    );
  }

  static String _statusLabel(AppLocalizations? l10n, SessionStatus status) =>
      switch (status) {
        SessionStatus.connecting => l10n?.connecting ?? 'Connecting…',
        SessionStatus.ready => l10n?.connected ?? 'Connected',
        SessionStatus.listening => l10n?.listening ?? 'Listening',
        SessionStatus.stopped => l10n?.stopped ?? 'Stopped',
        SessionStatus.failed => l10n?.connectionFailed ?? 'Connection failed',
      };
}

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message, required this.colors});

  final String message;
  final ParleyColors colors;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      color: colors.surface,
      padding: EdgeInsets.symmetric(
        horizontal: parleyMargin(context),
        vertical: 12,
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.error_outline, color: colors.error, size: 20),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: TextStyle(color: colors.error, fontSize: 14, height: 1.3),
            ),
          ),
        ],
      ),
    );
  }
}

/// Persistent outcome of a TTS failure: subtitles keep working, speech does
/// not. It stays until the next connection.
class _AudioUnavailableChip extends StatelessWidget {
  const _AudioUnavailableChip();

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    return Padding(
      padding: EdgeInsets.fromLTRB(
        parleyMargin(context),
        0,
        parleyMargin(context),
        8,
      ),
      child: Align(
        alignment: Alignment.centerLeft,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          decoration: BoxDecoration(
            color: colors.surface,
            borderRadius: BorderRadius.circular(999),
            border: Border.all(color: colors.outline),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.volume_off_outlined, size: 16, color: colors.muted),
              const SizedBox(width: 6),
              Flexible(
                child: Text(
                  l10n?.audioUnavailable ??
                      'Audio unavailable — subtitles only',
                  style: TextStyle(color: colors.muted, fontSize: 12),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _StartBar extends StatelessWidget {
  const _StartBar({required this.session});

  final ListenSession session;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    final listening = session.status == SessionStatus.listening;
    final canStart =
        session.status == SessionStatus.ready ||
        session.status == SessionStatus.stopped;
    return Container(
      color: colors.control,
      padding: EdgeInsets.fromLTRB(
        parleyMargin(context),
        8,
        parleyMargin(context),
        16,
      ),
      child: SizedBox(
        width: double.infinity,
        child: FilledButton(
          onPressed: listening
              ? session.stop
              : canStart
              ? session.start
              : null,
          style: FilledButton.styleFrom(
            backgroundColor: colors.ink,
            foregroundColor: colors.bg,
            disabledBackgroundColor: colors.hairline,
            disabledForegroundColor: colors.muted,
            minimumSize: const Size.fromHeight(56),
            textStyle: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
            ),
          ),
          child: Text(
            listening
                ? l10n?.stop ?? 'Stop'
                : l10n?.startListen ?? 'Start Listen',
          ),
        ),
      ),
    );
  }
}
