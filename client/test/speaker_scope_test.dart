import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/l10n/app_localizations.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/ui/speaker_scope.dart';

void main() {
  test(
    'a language resolves to its UI locale, unknown languages to English',
    () {
      final supported = AppLocalizations.supportedLocales;
      expect(localeForLanguage('ja', supported), const Locale('ja'));
      expect(localeForLanguage('zh-CN', supported), const Locale('zh'));
      expect(localeForLanguage('ZH', supported), const Locale('zh'));
      expect(localeForLanguage('fr', supported), const Locale('en'));
    },
  );

  testWidgets('each Converse half is localized to its speaker language', (
    tester,
  ) async {
    Widget label(String language, Key key) => SpeakerScope(
      language: language,
      child: Builder(
        builder: (context) =>
            Text(AppLocalizations.of(context).startListen, key: key),
      ),
    );

    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: parleyLocalizationDelegates,
        supportedLocales: parleySupportedLocales,
        home: Column(
          children: [
            label('ja', const Key('half-ja')),
            label('zh', const Key('half-zh')),
            label('fr', const Key('half-fr')),
          ],
        ),
      ),
    );

    expect(find.text('聞くを開始'), findsOneWidget);
    expect(find.text('开始聆听'), findsOneWidget);
    expect(find.text('Start Listen'), findsOneWidget);
  });
}
