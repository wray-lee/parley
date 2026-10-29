import 'package:material_ui/material_ui.dart';

import '../model/protocol.dart';
import '../theme.dart';

/// Committed Segments, newest large and low. A Segment is appended once and
/// never revised, so each entry is drawn from a value object.
class CaptionStack extends StatelessWidget {
  const CaptionStack({super.key, required this.segments, this.demo = false});

  final List<Segment> segments;
  final bool demo;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Column(
      children: [
        if (demo) const DemoBanner(),
        Expanded(
          child: segments.isEmpty
              ? Center(
                  child: Text(
                    'Captions land here.',
                    style: TextStyle(color: colors.muted, fontSize: 16),
                  ),
                )
              : ListView.builder(
                  reverse: true,
                  padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
                  itemCount: segments.length,
                  itemBuilder: (context, index) {
                    return Center(
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 720),
                        child: _SegmentView(
                          segment: segments[segments.length - 1 - index],
                          rank: index,
                          topGap: index == 0 ? 0 : 24,
                        ),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}

class DemoBanner extends StatelessWidget {
  const DemoBanner({super.key});

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Container(
      width: double.infinity,
      color: colors.demoBackground,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Text(
        'DEMO — sample audio, mock providers',
        style: TextStyle(
          color: colors.demoForeground,
          fontSize: 14,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}

class _SegmentView extends StatelessWidget {
  const _SegmentView({
    required this.segment,
    required this.rank,
    required this.topGap,
  });

  final Segment segment;

  /// 0 = newest, 1 = previous, 2+ = older.
  final int rank;
  final double topGap;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final newest = rank == 0;
    final previous = rank == 1;
    return Padding(
      padding: EdgeInsets.only(top: topGap),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            segment.source,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(color: colors.muted, fontSize: 14, height: 1.3),
          ),
          const SizedBox(height: 4),
          Text(
            segment.translation,
            style: TextStyle(
              color: newest || previous ? colors.ink : colors.muted,
              fontSize: newest ? 28 : (previous ? 20 : 18),
              fontWeight: newest ? FontWeight.w600 : FontWeight.w400,
              height: 1.25,
            ),
          ),
        ],
      ),
    );
  }
}
