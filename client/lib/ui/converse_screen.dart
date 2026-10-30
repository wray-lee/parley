import 'dart:async';

import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform, kIsWeb;
import 'package:flutter/services.dart' show HapticFeedback;
import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../model/protocol.dart';
import '../platform/keep_awake.dart';
import '../session/listen_session.dart';
import '../theme.dart';
import 'caption_stack.dart';
import 'language_names.dart';
import 'live_line.dart';
import 'session_bar.dart';
import 'speaker_scope.dart';

/// Converse: two speakers share one phone. A phone in portrait splits the
/// screen and rotates the far half 180°; large text, landscape, web/desktop
/// and screen readers get the stacked "pass the phone" layout instead. Each
/// half is scoped to its speaker's language.
class ConverseScreen extends StatefulWidget {
  const ConverseScreen({
    super.key,
    required this.session,
    this.wakeLock = keepAwake,
    this.onSettings,
    this.onUseKeys,
    this.platform,
    this.isWeb,
  });

  final ListenSession session;
  final Future<void> Function(bool on) wakeLock;

  /// Opens Settings from my half; the read-only web build still shows it.
  final VoidCallback? onSettings;

  /// The Demo banner's "Use my keys" CTA; null on read-only web settings,
  /// where there is no key field to reach.
  final VoidCallback? onUseKeys;

  /// Test seams for the layout choice; production reads the real platform.
  final TargetPlatform? platform;
  final bool? isWeb;

  @override
  State<ConverseScreen> createState() => _ConverseScreenState();
}

class _ConverseScreenState extends State<ConverseScreen> {
  int _seenCue = 0;
  String? _cueWho;
  Timer? _cueTimer;
  bool _awake = false;

  @override
  void initState() {
    super.initState();
    _seenCue = widget.session.turnCue;
    widget.session.addListener(_onSession);
    _syncAwake();
  }

  @override
  void dispose() {
    widget.session.removeListener(_onSession);
    _cueTimer?.cancel();
    unawaited(_setAwake(false));
    super.dispose();
  }

  void _syncAwake() {
    unawaited(_setAwake(widget.session.status == SessionStatus.listening));
  }

  Future<void> _setAwake(bool on) async {
    if (_awake == on) return;
    _awake = on;
    try {
      await widget.wakeLock(on);
    } catch (error) {
      debugPrint('PARLEY wake lock $error');
    }
  }

  /// A Turn change after the first: "Your turn" on the new speaker's half for
  /// a second, plus a haptic. Motion is 250 ms, or none when the OS asks.
  void _onSession() {
    _syncAwake();
    if (widget.session.turnCue == _seenCue) return;
    _seenCue = widget.session.turnCue;
    final who = widget.session.turnCueWho;
    if (who == null) return;
    _cueTimer?.cancel();
    setState(() => _cueWho = who);
    HapticFeedback.lightImpact();
    _cueTimer = Timer(const Duration(seconds: 1), () {
      if (mounted) setState(() => _cueWho = null);
    });
  }

  /// Split only for a phone in portrait: web, desktop, tablets, landscape,
  /// text above 1.5x and screen readers all stay upright and stacked.
  bool _useSplit(BuildContext context) {
    if (widget.isWeb ?? kIsWeb) return false;
    final platform = widget.platform ?? defaultTargetPlatform;
    if (platform != TargetPlatform.android && platform != TargetPlatform.iOS) {
      return false;
    }
    final media = MediaQuery.of(context);
    if (media.accessibleNavigation) return false;
    if (media.size.width >= media.size.height) return false;
    if (media.size.shortestSide >= 600) return false;
    return media.textScaler.scale(1) <= 1.5;
  }

  Duration _cueDuration(BuildContext context) =>
      MediaQuery.of(context).disableAnimations
      ? Duration.zero
      : const Duration(milliseconds: 250);

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.session,
      builder: (context, _) {
        final session = widget.session;
        final split = _useSplit(context);
        final l10n = AppLocalizations.of(context);
        final banners = [
          if (session.demo) DemoBanner(onUseKeys: widget.onUseKeys),
          if (session.error != null) _ErrorBanner(message: session.error!),
          if (session.audioError != null) const AudioUnavailableChip(),
        ];
        return Scaffold(
          backgroundColor: ParleyColors.of(context).bg,
          body: SafeArea(
            child: Column(
              children: [
                for (final banner in banners)
                  MediaQuery.withClampedTextScaling(
                    maxScaleFactor: 1.3,
                    child: banner,
                  ),
                Expanded(
                  child: split
                      ? Column(
                          key: const Key('converse-split'),
                          children: [
                            Expanded(
                              child: RotatedBox(
                                key: const Key('converse-rotation'),
                                quarterTurns: 2,
                                child: _half('them'),
                              ),
                            ),
                            Expanded(child: _half('me')),
                          ],
                        )
                      : Column(
                          key: const Key('converse-stacked'),
                          children: [
                            Expanded(child: _half('me')),
                            Expanded(child: _half('them')),
                          ],
                        ),
                ),
                MediaQuery.withClampedTextScaling(
                  maxScaleFactor: 1.3,
                  child: SessionStartBar(
                    session: session,
                    startLabel: l10n.startConverse,
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget _half(String who) {
    final session = widget.session;
    final me = session.meDirection;
    final them = session.themDirection;
    if (me == null || them == null) {
      if (session.status == SessionStatus.connecting) {
        return const Center(child: CircularProgressIndicator());
      }
      return Center(
        child: Text(
          AppLocalizations.of(context).captionsEmpty,
          style: TextStyle(color: ParleyColors.of(context).muted, fontSize: 16),
        ),
      );
    }
    // My speech is src->tgt, so my language is the source of my Direction and
    // theirs is its target. A half shows the Segments translated into it.
    final language = who == 'me' ? me.src : me.tgt;
    final captions = session.segments
        .where((segment) => segment.direction?.tgt == language)
        .toList(growable: false);
    return SpeakerScope(
      language: language,
      child: Builder(
        builder: (context) {
          final l10n = AppLocalizations.of(context);
          final colors = ParleyColors.of(context);
          final speaker = who == 'me' ? l10n.converseMe : l10n.converseThem;
          final active = session.turn == who;
          return Semantics(
            container: true,
            header: true,
            label: '$speaker · ${languageName(context, language)}',
            child: LayoutBuilder(
              builder: (context, constraints) {
                final compact = constraints.maxHeight < 300;
                return Container(
                  key: Key('half-$who'),
                  decoration: BoxDecoration(
                    border: Border(top: BorderSide(color: colors.hairline)),
                  ),
                  padding: EdgeInsets.fromLTRB(16, compact ? 8 : 12, 16, 8),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      MediaQuery.withClampedTextScaling(
                        maxScaleFactor: 1.3,
                        child: _header(
                          context,
                          who,
                          speaker,
                          language,
                          compact,
                        ),
                      ),
                      Expanded(
                        child: _CaptionList(
                          segments: captions,
                          // Gating, the live line and the Turn cue scroll with
                          // the captions, so tall text never clips the pinned
                          // Turn pad.
                          footer: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              if (session.turn == who &&
                                  (!session.live.isEmpty ||
                                      session.held ||
                                      session.inlineError != null))
                                LiveLine(
                                  caption: session.live,
                                  held: session.held,
                                  merges: session.merges,
                                  error: session.inlineError,
                                  advanced: true,
                                  onSkip: session.skipLive,
                                ),
                              if (session.gated)
                                Padding(
                                  padding: const EdgeInsets.only(top: 4),
                                  child: Row(
                                    children: [
                                      Icon(
                                        Icons.mic_off_outlined,
                                        size: 14,
                                        color: colors.muted,
                                      ),
                                      const SizedBox(width: 6),
                                      Flexible(
                                        child: Text(
                                          l10n.converseMicPaused,
                                          maxLines: 1,
                                          overflow: TextOverflow.ellipsis,
                                          style: TextStyle(
                                            color: colors.muted,
                                            fontSize: 12,
                                          ),
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                              AnimatedSwitcher(
                                key: Key('turn-cue-$who'),
                                duration: _cueDuration(context),
                                child: _cueWho == who
                                    ? Text(
                                        l10n.converseYourTurn,
                                        key: ValueKey('cue-$who'),
                                        style: TextStyle(
                                          color: colors.liveText,
                                          fontSize: 14,
                                          fontWeight: FontWeight.w600,
                                        ),
                                      )
                                    : const SizedBox.shrink(
                                        key: ValueKey('cue-none'),
                                      ),
                              ),
                            ],
                          ),
                        ),
                      ),
                      const SizedBox(height: 4),
                      _TurnPad(
                        who: who,
                        speaker: speaker,
                        active: active,
                        size: compact ? 64 : 96,
                        duration: _cueDuration(context),
                        onTap: () => session.takeTurn(who),
                        onHoldStart: () => session.beginPushToTalk(who),
                        onHoldEnd: session.endPushToTalk,
                      ),
                    ],
                  ),
                );
              },
            ),
          );
        },
      ),
    );
  }

  Widget _header(
    BuildContext context,
    String who,
    String speaker,
    String language,
    bool compact,
  ) {
    final colors = ParleyColors.of(context);
    final l10n = AppLocalizations.of(context);
    return Row(
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                speaker,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: colors.ink,
                  fontSize: compact ? 14 : 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              Text(
                languageName(context, language),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(color: colors.muted, fontSize: 12),
              ),
            ],
          ),
        ),
        if (who == 'me') ...[
          IconButton(
            key: const Key('converse-settings'),
            tooltip: l10n.settings,
            onPressed: widget.onSettings,
            icon: Icon(Icons.settings_outlined, color: colors.muted),
          ),
        ],
      ],
    );
  }
}

class _TurnPad extends StatefulWidget {
  const _TurnPad({
    required this.who,
    required this.speaker,
    required this.active,
    required this.size,
    required this.duration,
    required this.onTap,
    required this.onHoldStart,
    required this.onHoldEnd,
  });

  final String who;
  final String speaker;
  final bool active;
  final double size;
  final Duration duration;
  final VoidCallback onTap;
  final VoidCallback onHoldStart;
  final VoidCallback onHoldEnd;

  @override
  State<_TurnPad> createState() => _TurnPadState();
}

class _TurnPadState extends State<_TurnPad> {
  bool _holding = false;

  void _startHold() {
    _holding = true;
    widget.onHoldStart();
  }

  /// LongPressGestureRecognizer drops an accepted gesture on a pointer
  /// cancel without calling onLongPressEnd, so the raw pointer events finish
  /// the hold too.
  void _endHold() {
    if (!_holding) return;
    _holding = false;
    widget.onHoldEnd();
  }

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = AppLocalizations.of(context);
    return Semantics(
      button: true,
      label:
          '${widget.speaker}. ${l10n.converseTapToTake}, '
          '${l10n.converseHoldToTalk}.',
      child: Listener(
        onPointerUp: (_) => _endHold(),
        onPointerCancel: (_) => _endHold(),
        child: GestureDetector(
          key: Key('pad-${widget.who}'),
          onTap: widget.onTap,
          onLongPressStart: (_) => _startHold(),
          onLongPressEnd: (_) => _endHold(),
          child: AnimatedContainer(
            key: Key('pad-surface-${widget.who}'),
            duration: widget.duration,
            width: widget.size,
            height: widget.size,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: widget.active ? colors.live : colors.control,
              border: Border.all(
                color: widget.active ? colors.live : colors.outline,
                width: 1,
              ),
            ),
            child: Icon(
              widget.active ? Icons.mic : Icons.mic_none,
              color: widget.active ? colors.onLive : colors.muted,
              size: widget.size * 0.4,
            ),
          ),
        ),
      ),
    );
  }
}

/// One half's committed Segments, newest low. A new Segment jumps back to
/// live so the newest subtitle never lands below the fold.
class _CaptionList extends StatefulWidget {
  const _CaptionList({required this.segments, this.footer});

  final List<Segment> segments;

  /// Uncommitted status shown under the newest Segment, inside the same
  /// scroll view.
  final Widget? footer;

  @override
  State<_CaptionList> createState() => _CaptionListState();
}

class _CaptionListState extends State<_CaptionList> {
  final ScrollController _controller = ScrollController();

  @override
  void didUpdateWidget(_CaptionList old) {
    super.didUpdateWidget(old);
    if (widget.segments.length > old.segments.length) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && _controller.hasClients) _controller.jumpTo(0);
      });
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final segments = widget.segments;
    final footer = widget.footer;
    final footerCount = footer == null ? 0 : 1;
    return ListView.builder(
      controller: _controller,
      reverse: true,
      padding: const EdgeInsets.symmetric(vertical: 8),
      itemCount: segments.length + footerCount,
      itemBuilder: (context, index) {
        if (footer != null && index == 0) return footer;
        return _CaptionTile(
          segment: segments[segments.length - 1 - (index - footerCount)],
        );
      },
    );
  }
}

class _CaptionTile extends StatelessWidget {
  const _CaptionTile({required this.segment});

  final Segment segment;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final direction = segment.direction;
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            segment.translation,
            style: TextStyle(
              color: colors.ink,
              fontSize: 24,
              height: 1.25,
              locale: direction == null ? null : Locale(direction.tgt),
            ),
          ),
          if (segment.source.isNotEmpty)
            Text(
              segment.source,
              style: TextStyle(
                color: colors.muted,
                fontSize: 14,
                locale: direction == null ? null : Locale(direction.src),
              ),
            ),
        ],
      ),
    );
  }
}

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Container(
      width: double.infinity,
      color: colors.control,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Text(message, style: TextStyle(color: colors.error, fontSize: 14)),
    );
  }
}
