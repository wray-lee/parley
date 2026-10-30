import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../session/listen_session.dart';
import '../theme.dart';

/// The Start/Stop bar under a Session surface. One control, always present:
/// a stopped or failed Session is never a dead end.
class SessionStartBar extends StatelessWidget {
  const SessionStartBar({
    super.key,
    required this.session,
    required this.startLabel,
  });

  final ListenSession session;
  final String startLabel;

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
          key: const Key('session-bar'),
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
          child: Text(listening ? l10n?.stop ?? 'Stop' : startLabel),
        ),
      ),
    );
  }
}
