import '../l10n/app_localizations.dart';

/// The next action under an inline Provider error, chosen by the core's error
/// `kind`. `network`, `model` and `cors` are accepted as aliases so a host
/// that names the failure at the browser boundary gets the same recovery.
String? errorNextAction(AppLocalizations l10n, String? kind) => switch (kind) {
  'auth' => l10n.errorNextAuth,
  'quota' || 'rate_limit' => l10n.errorNextQuota,
  'network' || 'unreachable' => l10n.errorNextNetwork,
  'model' || 'config' => l10n.errorNextModel,
  'cors' => l10n.errorNextCors,
  'timeout' => l10n.errorNextTimeout,
  'protocol' => l10n.errorNextGeneric,
  _ => null,
};

/// Kinds whose next action is a Settings edit: the error row offers the link.
bool errorOffersSettings(String? kind) =>
    kind == 'auth' || kind == 'model' || kind == 'config';
