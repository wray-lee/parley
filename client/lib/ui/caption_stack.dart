import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../model/protocol.dart';
import '../theme.dart';
import 'live_line.dart';

/// Committed Segments, newest large and low, with the live Partial under them.
/// A Segment is appended once and never revised, so each entry is drawn from a
/// value object; only Committed translations are a polite live region.
class CaptionStack extends StatefulWidget {
  const CaptionStack({
    super.key,
    required this.segments,
    this.demo = false,
    this.live = LiveCaption.empty,
    this.held = false,
    this.merges = 0,
    this.inlineError,
    this.captionSize = 28,
    this.showSource = true,
    this.showLatency = false,
    this.advanced = false,
    this.focus = false,
    this.onSkipLive,
    this.onUseKeys,
  });

  final List<Segment> segments;
  final bool demo;
  final LiveCaption live;
  final bool held;
  final int merges;
  final String? inlineError;

  /// The Aa step: 24/28/34/40. Captions are never clamped; the OS text scale
  /// applies on top of this.
  final double captionSize;
  final bool showSource;
  final bool showLatency;
  final bool advanced;

  /// Focus mode: hide the source, and tapping the captions pauses following.
  final bool focus;
  final VoidCallback? onSkipLive;

  /// The Demo banner CTA; null hides it (read-only web).
  final VoidCallback? onUseKeys;

  @override
  State<CaptionStack> createState() => _CaptionStackState();
}

class _CaptionStackState extends State<CaptionStack> {
  final ScrollController _controller = ScrollController();
  bool _followLive = true;
  bool _jumpingToLive = false;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onScroll);
  }

  @override
  void didUpdateWidget(CaptionStack old) {
    super.didUpdateWidget(old);
    if (widget.segments.length > old.segments.length && _followLive) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && _controller.hasClients) {
          _controller.animateTo(
            0,
            duration: const Duration(milliseconds: 200),
            curve: Curves.easeOut,
          );
        }
      });
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _onScroll() {
    if (_jumpingToLive) return;
    final atLive = !_controller.hasClients || _controller.offset <= 8;
    if (atLive && !_followLive) {
      setState(() => _followLive = true);
    } else if (!atLive && _followLive) {
      setState(() => _followLive = false);
    }
  }

  void _toggleFollow() {
    setState(() => _followLive = !_followLive);
    if (_followLive) _jumpToLive();
  }

  Future<void> _jumpToLive() async {
    if (!_followLive) setState(() => _followLive = true);
    if (!_controller.hasClients) return;
    _jumpingToLive = true;
    await _controller.animateTo(
      0,
      duration: const Duration(milliseconds: 250),
      curve: Curves.easeOut,
    );
    _jumpingToLive = false;
  }

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    final empty =
        widget.segments.isEmpty &&
        widget.live.isEmpty &&
        widget.inlineError == null;
    Widget body = Stack(
      children: [
        if (empty)
          Center(
            child: Text(
              l10n?.captionsEmpty ?? 'Captions land here.',
              style: TextStyle(color: colors.muted, fontSize: 16),
            ),
          )
        else
          ListView.builder(
            controller: _controller,
            reverse: true,
            padding: EdgeInsets.fromLTRB(
              parleyMargin(context),
              16,
              parleyMargin(context),
              8,
            ),
            itemCount: widget.segments.length,
            itemBuilder: (context, index) {
              return Center(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 720),
                  // Fill the column so short lines align to the margin, not
                  // to the centre of their own text.
                  child: SizedBox(
                    width: double.infinity,
                    child: _SegmentView(
                      segment:
                          widget.segments[widget.segments.length - 1 - index],
                      rank: index,
                      topGap: index == 0 ? 0 : 24,
                      captionSize: widget.captionSize,
                      showSource: widget.showSource,
                      showLatency: widget.showLatency,
                    ),
                  ),
                ),
              );
            },
          ),
        if (!_followLive)
          Positioned(
            left: 0,
            right: 0,
            bottom: 8,
            child: Center(
              child: TextButton(
                key: const Key('jump-to-live'),
                onPressed: _jumpToLive,
                style: TextButton.styleFrom(
                  minimumSize: const Size(48, 48),
                  backgroundColor: colors.control,
                  foregroundColor: colors.ink,
                ),
                child: Text(l10n?.jumpToLive ?? 'Jump to live'),
              ),
            ),
          ),
      ],
    );
    if (widget.focus) {
      body = Semantics(
        container: true,
        button: true,
        label: _followLive
            ? l10n?.pauseCaptions ?? 'Pause captions'
            : l10n?.resumeCaptions ?? 'Resume captions',
        onTap: _toggleFollow,
        child: GestureDetector(
          behavior: HitTestBehavior.translucent,
          onTap: _toggleFollow,
          child: body,
        ),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (widget.demo) DemoBanner(onUseKeys: widget.onUseKeys),
        Expanded(child: body),
        LiveLine(
          caption: widget.live,
          held: widget.held,
          merges: widget.merges,
          error: widget.inlineError,
          advanced: widget.advanced,
          onSkip: widget.onSkipLive,
        ),
      ],
    );
  }
}

class DemoBanner extends StatelessWidget {
  const DemoBanner({super.key, this.onUseKeys});

  /// The Demo Mode CTA; null hides it (read-only web settings).
  final VoidCallback? onUseKeys;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    return Container(
      key: const Key('demo-banner'),
      width: double.infinity,
      color: colors.demoBackground,
      padding: EdgeInsets.fromLTRB(parleyMargin(context), 8, 8, 8),
      child: Row(
        children: [
          Icon(Icons.science_outlined, size: 18, color: colors.demoForeground),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              l10n?.demoBanner ?? 'DEMO — sample audio, mock providers',
              style: TextStyle(
                color: colors.demoForeground,
                fontSize: 14,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          if (onUseKeys != null)
            TextButton(
              onPressed: onUseKeys,
              child: Text(
                l10n?.demoUseMyKeys ?? 'Use my keys',
                style: TextStyle(color: colors.demoForeground),
              ),
            ),
        ],
      ),
    );
  }
}

class _SegmentView extends StatelessWidget {
  const _SegmentView({
    required this.segment,
    required this.rank,
    required this.topGap,
    required this.captionSize,
    required this.showSource,
    required this.showLatency,
  });

  final Segment segment;

  /// 0 = newest, 1 = previous, 2+ = older.
  final int rank;
  final double topGap;
  final double captionSize;
  final bool showSource;
  final bool showLatency;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final newest = rank == 0;
    final previous = rank == 1;
    final size = newest
        ? captionSize
        : (previous ? captionSize * 20 / 28 : captionSize * 18 / 28);
    final direction = segment.direction;
    final sourceLocale = direction == null ? null : Locale(direction.src);
    final translationLocale = direction == null ? null : Locale(direction.tgt);
    return Padding(
      padding: EdgeInsets.only(top: topGap),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (showSource)
            Text(
              segment.source,
              style: TextStyle(
                color: colors.muted,
                fontSize: 14,
                height: 1.3,
                locale: sourceLocale,
              ),
            ),
          if (showSource) const SizedBox(height: 4),
          Semantics(
            key: ValueKey('segment-live-${segment.segId}'),
            container: true,
            liveRegion: true,
            label: segment.translation,
            child: ExcludeSemantics(
              child: Text(
                segment.translation,
                style: TextStyle(
                  color: newest || previous ? colors.ink : colors.muted,
                  fontSize: size,
                  fontWeight: newest ? FontWeight.w600 : FontWeight.w400,
                  height: 1.25,
                  locale: translationLocale,
                ),
              ),
            ),
          ),
          if (showLatency)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(
                '${segment.latencyMs} ms',
                style: TextStyle(
                  color: colors.muted,
                  fontSize: 12,
                  fontFeatures: const [FontFeature.tabularFigures()],
                ),
              ),
            ),
        ],
      ),
    );
  }
}
