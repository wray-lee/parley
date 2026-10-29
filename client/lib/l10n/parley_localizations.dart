import 'package:material_ui/material_ui.dart';

import 'app_localizations.dart';

/// The generated delegate list targets `package:flutter/material.dart`, but
/// this build draws Material and Cupertino widgets from `material_ui` and
/// `cupertino_ui`, whose localization types are distinct. Their delegates make
/// the Material strings follow zh/ja instead of the English-only defaults.
const List<LocalizationsDelegate<dynamic>> parleyLocalizationDelegates = [
  AppLocalizations.delegate,
  ...GlobalMaterialLocalizations.delegates,
];

const List<Locale> parleySupportedLocales = AppLocalizations.supportedLocales;
