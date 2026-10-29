import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';

/// The localized name of a Language Pair end. Display data only: the core
/// never sees these strings.
String languageName(BuildContext context, String language) {
  final l10n = AppLocalizations.of(context);
  return switch (language.toLowerCase().split('-').first) {
    'en' => l10n.languageEnglish,
    'zh' => l10n.languageChinese,
    'ja' => l10n.languageJapanese,
    _ => language.toUpperCase(),
  };
}
