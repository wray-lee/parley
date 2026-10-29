import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';

/// The UI locale for one speaker's language. Exact matches win; a language
/// without UI strings falls back to English, keeping the icons as the shared
/// affordance.
Locale localeForLanguage(String language, List<Locale> supported) {
  final languageCode = language
      .replaceAll('_', '-')
      .split('-')
      .first
      .toLowerCase();
  for (final locale in supported) {
    if (locale.languageCode.toLowerCase() == languageCode) return locale;
  }
  for (final locale in supported) {
    if (locale.languageCode == 'en') return locale;
  }
  return const Locale('en');
}

/// Scopes a Converse half's strings to its speaker's language. Ticket 15 wraps
/// each half in one of these; an unknown language speaks English.
class SpeakerScope extends StatelessWidget {
  const SpeakerScope({super.key, required this.language, required this.child});

  final String language;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Localizations.override(
      context: context,
      locale: localeForLanguage(language, AppLocalizations.supportedLocales),
      child: child,
    );
  }
}
