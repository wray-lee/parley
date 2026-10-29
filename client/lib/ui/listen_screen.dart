import 'package:material_ui/material_ui.dart';

import '../model/protocol.dart';
import '../session/listen_session.dart';
import '../theme.dart';
import 'caption_stack.dart';

/// The Listen surface: status and pair from `hello`, the caption stack, and
/// one Start/Stop bar. Connection details live behind [ListenSession].
class ListenScreen extends StatelessWidget {
  const ListenScreen({super.key, required this.session});

  final ListenSession session;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: session,
      builder: (context, _) {
        final colors = ParleyColors.of(context);
        return Scaffold(
          body: SafeArea(
            child: Column(
              children: [
                _Header(session: session, colors: colors),
                if (session.error != null)
                  _ErrorBanner(message: session.error!, colors: colors),
                if (session.audioError != null)
                  _AudioNotice(message: session.audioError!),
                Expanded(
                  child: CaptionStack(
                    segments: session.segments,
                    demo: session.demo,
                  ),
                ),
                _StartBar(session: session),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.session, required this.colors});

  final ListenSession session;
  final ParleyColors colors;

  @override
  Widget build(BuildContext context) {
    final listening = session.status == SessionStatus.listening;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
      child: Row(
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
          Text(
            _statusLabel(session.status),
            style: TextStyle(
              color: listening ? colors.liveText : colors.muted,
              fontSize: 14,
            ),
          ),
          const Spacer(),
          if (session.pairs.isNotEmpty)
            DropdownButton<LanguagePair>(
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
        ],
      ),
    );
  }

  static String _statusLabel(SessionStatus status) => switch (status) {
    SessionStatus.connecting => 'Connecting…',
    SessionStatus.ready => 'Connected',
    SessionStatus.listening => 'Listening',
    SessionStatus.stopped => 'Stopped',
    SessionStatus.failed => 'Connection failed',
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
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
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

class _AudioNotice extends StatelessWidget {
  const _AudioNotice({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
      child: Align(
        alignment: Alignment.centerLeft,
        child: Text(
          message,
          style: TextStyle(color: colors.muted, fontSize: 12),
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
    final listening = session.status == SessionStatus.listening;
    final canStart =
        session.status == SessionStatus.ready ||
        session.status == SessionStatus.stopped;
    return Container(
      color: colors.control,
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
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
          child: Text(listening ? 'Stop' : 'Start Listen'),
        ),
      ),
    );
  }
}
