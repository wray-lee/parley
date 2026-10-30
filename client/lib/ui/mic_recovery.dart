import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../platform/mic_permission_api.dart';
import '../session/listen_session.dart';
import '../theme.dart';

/// Shown after a Start was refused by the microphone: a Settings deep link
/// for a denial, and the HTTPS explanation when the page is not secure.
class MicRecoveryCard extends StatelessWidget {
  const MicRecoveryCard({super.key, required this.session});

  final ListenSession session;

  @override
  Widget build(BuildContext context) {
    final access = session.micRecovery;
    if (access == null || access == MicAccess.granted) {
      return const SizedBox.shrink();
    }
    final colors = ParleyColors.of(context);
    final l10n = Localizations.of<AppLocalizations>(context, AppLocalizations);
    final insecure = access == MicAccess.insecure;
    return Container(
      key: const Key('mic-recovery-card'),
      width: double.infinity,
      margin: EdgeInsets.fromLTRB(
        parleyMargin(context),
        8,
        parleyMargin(context),
        0,
      ),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.outline),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.mic_off_outlined, size: 18, color: colors.error),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  insecure
                      ? l10n?.micInsecure ?? 'The microphone needs HTTPS. Open Parley over https:// or on localhost.'
                      : l10n?.micDenied ?? 'Microphone access is off. Allow it in Settings, then Start again.',
                  style: TextStyle(color: colors.ink, fontSize: 14),
                ),
              ),
            ],
          ),
          if (!insecure)
            Align(
              alignment: Alignment.centerRight,
              child: TextButton(
                key: const Key('mic-recovery-settings'),
                onPressed: session.openMicSettings,
                child: Text(l10n?.openSettings ?? 'Open Settings'),
              ),
            ),
        ],
      ),
    );
  }
}
