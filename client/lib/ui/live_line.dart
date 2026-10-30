import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../model/protocol.dart';
import '../theme.dart';
import 'error_action.dart';

/// The line under the newest Committed Segment. It shows the Partial (Stable
/// Prefix upright, unstable tail provisional) and the Hold/Slow/Error states.
/// Nothing here is a committed Segment, so the whole Partial and Hold line is
/// hidden from screen readers.
class LiveLine extends StatefulWidget {
  const LiveLine({
    super.key,
    required this.caption,
    this.held = false,
    this.merges = 0,
    this.error,
    this.errorKind,
    this.advanced = false,
    this.onSkip,
    this.onSettings,
  });

  final LiveCaption caption;
  final bool held;
  final int merges;
  final String? error;

  /// The core's error kind; it selects the next action under [error].
  final String? errorKind;
  final bool advanced;
  final VoidCallback? onSkip;
  final VoidCallback? onSettings;

  @override
  State<LiveLine> createState() => _LiveLineState();
}

class _LiveLineState extends State<LiveLine> {
  Timer? _holdTimer;
  Timer? _forcedTimer;
  Timer? _slowTimer;
  bool _holdLong = false;
  bool _forced = false;
  bool _slow = false;

  @override
  void initState() {
    super.initState();
    if (widget.held) _armHold();
    _armSlow();
  }

  @override
  void didUpdateWidget(LiveLine old) {
    super.didUpdateWidget(old);
    if (widget.held && !old.held) _armHold();
    if (!widget.held && old.held) _cancelHold();
    if (old.caption.text != widget.caption.text ||
        old.held != widget.held ||
        old.error != widget.error) {
      _armSlow();
    }
  }

  @override
  void dispose() {
    _cancelHold();
    _slowTimer?.cancel();
    super.dispose();
  }

  void _armHold() {
    _cancelHold();
    _holdTimer = Timer(const Duration(milliseconds: 1500), () {
      if (mounted) setState(() => _holdLong = true);
    });
    _forcedTimer = Timer(const Duration(milliseconds: 2500), () {
      if (mounted) setState(() => _forced = true);
    });
    _slowTimer?.cancel();
    _slow = false;
  }

  void _cancelHold() {
    _holdTimer?.cancel();
    _forcedTimer?.cancel();
    _holdTimer = null;
    _forcedTimer = null;
    _holdLong = false;
    _forced = false;
  }

  /// A translation that has not committed five seconds after the last Partial
  /// reads as slow. The wire has no "translating" event, so the Partial is the
  /// only pending signal the client has.
  void _armSlow() {
    _slowTimer?.cancel();
    _slow = false;
    if (widget.caption.isEmpty || widget.held || widget.error != null) return;
    _slowTimer = Timer(const Duration(seconds: 5), () {
      if (mounted) setState(() => _slow = true);
    });
  }

  // shortcut: the wire has no translator-retry message, so Retry re-arms the
  // slow timer and keeps waiting; revisit when the protocol grows one.
  void _retry() {
    setState(_armSlow);
  }

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    final caption = widget.caption;
    final margin = parleyMargin(context);
    return Padding(
      padding: EdgeInsets.fromLTRB(margin, 8, margin, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (!caption.isEmpty)
            ExcludeSemantics(
              child: Text.rich(
                key: const Key('live-partial'),
                TextSpan(
                  children: [
                    TextSpan(
                      text: caption.stable,
                      style: TextStyle(color: colors.ink),
                    ),
                    TextSpan(
                      text: caption.provisional,
                      style: TextStyle(color: colors.provisional),
                    ),
                  ],
                ),
                style: const TextStyle(fontSize: 16, height: 1.35),
              ),
            ),
          if (widget.held) _holdStatus(colors, l10n),
          if (!widget.held && widget.error == null && _slow)
            _SlowRow(onRetry: _retry, onSkip: widget.onSkip),
          if (widget.error != null) _errorRow(colors, l10n),
        ],
      ),
    );
  }

  /// The message plus the kind's next action; auth and model errors also
  /// offer the Settings link, because that is where the fix lives.
  Widget _errorRow(ParleyColors colors, AppLocalizations? l10n) {
    final next = l10n == null ? null : errorNextAction(l10n, widget.errorKind);
    final settings = errorOffersSettings(widget.errorKind);
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            widget.error!,
            style: TextStyle(color: colors.error, fontSize: 14),
          ),
          if (next != null)
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      next,
                      key: const Key('error-next-action'),
                      style: TextStyle(color: colors.muted, fontSize: 14),
                    ),
                  ),
                  if (settings && widget.onSettings != null)
                    TextButton(
                      key: const Key('error-open-settings'),
                      onPressed: widget.onSettings,
                      child: Text(l10n?.openSettings ?? 'Open Settings'),
                    ),
                ],
              ),
            ),
        ],
      ),
    );
  }

  Widget _holdStatus(ParleyColors colors, AppLocalizations? l10n) {
    final merges = widget.advanced && widget.merges > 0
        ? ' · ${widget.merges} ${widget.merges == 1 ? 'merge' : 'merges'}'
        : '';
    final style = TextStyle(color: colors.muted, fontSize: 14);
    if (_forced) {
      return ExcludeSemantics(
        child: Padding(
          padding: const EdgeInsets.only(top: 4),
          child: Text(
            '${l10n?.finishing ?? 'finishing…'}$merges',
            style: style,
          ),
        ),
      );
    }
    if (_holdLong) {
      return ExcludeSemantics(
        child: Padding(
          padding: const EdgeInsets.only(top: 4),
          child: _Pulse(
            child: Text(
              '${l10n?.stillListening ?? 'still listening…'}$merges',
              style: style,
            ),
          ),
        ),
      );
    }
    return ExcludeSemantics(
      child: Padding(
        padding: const EdgeInsets.only(top: 4),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.pause_rounded, size: 16, color: colors.muted),
            if (merges.isNotEmpty) Text(merges, style: style),
          ],
        ),
      ),
    );
  }
}

class _SlowRow extends StatelessWidget {
  const _SlowRow({this.onRetry, this.onSkip});

  final VoidCallback? onRetry;
  final VoidCallback? onSkip;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: Wrap(
        crossAxisAlignment: WrapCrossAlignment.center,
        spacing: 8,
        children: [
          Text(
            l10n?.takingLonger ?? 'taking longer than usual',
            style: TextStyle(color: colors.muted, fontSize: 14),
          ),
          TextButton(
            onPressed: onRetry,
            style: TextButton.styleFrom(
              minimumSize: const Size(48, 48),
              foregroundColor: colors.ink,
            ),
            child: Text(l10n?.retry ?? 'Retry'),
          ),
          TextButton(
            onPressed: onSkip,
            style: TextButton.styleFrom(
              minimumSize: const Size(48, 48),
              foregroundColor: colors.ink,
            ),
            child: Text(l10n?.skip ?? 'Skip'),
          ),
        ],
      ),
    );
  }
}

/// Fades its child in and out unless the OS asks for reduced motion, in which
/// case it is a plain static child.
class _Pulse extends StatefulWidget {
  const _Pulse({required this.child});

  final Widget child;

  @override
  State<_Pulse> createState() => _PulseState();
}

class _PulseState extends State<_Pulse> with SingleTickerProviderStateMixin {
  AnimationController? _controller;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _controller?.dispose();
      _controller = null;
    } else {
      _controller ??= AnimationController(
        vsync: this,
        duration: const Duration(milliseconds: 900),
      )..repeat(reverse: true);
    }
  }

  @override
  void dispose() {
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final controller = _controller;
    if (controller == null) return widget.child;
    return FadeTransition(
      opacity: Tween<double>(
        begin: 0.35,
        end: 1,
      ).animate(CurvedAnimation(parent: controller, curve: Curves.easeInOut)),
      child: widget.child,
    );
  }
}
