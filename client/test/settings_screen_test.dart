import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/settings/presets.dart';
import 'package:parley/settings/secret_store.dart';
import 'package:parley/settings/settings_controller.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/settings_screen.dart';

import 'fakes.dart';

Widget wrap(Widget child) => MaterialApp(
  theme: parleyTheme(Brightness.dark),
  localizationsDelegates: parleyLocalizationDelegates,
  supportedLocales: parleySupportedLocales,
  home: child,
);

Future<
  ({
    ListenSession session,
    SettingsController controller,
    MemorySecretStore store,
    FakeTransport transport,
  })
>
make() async {
  final transport = FakeTransport();
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
  );
  final store = MemorySecretStore({
    SettingsController.llmSecretKey: 'sk-llm-1234abcd',
  });
  // The real catalog: it contains `custom` as a real Preset, which is exactly
  // the shape that used to crash the dropdown. Read synchronously; a widget
  // test cannot await rootBundle before its first pump.
  final controller = SettingsController(
    store: store,
    presets: PresetCatalog.fromJson(
      jsonDecode(File('../config/presets.json').readAsStringSync())
          as Map<String, dynamic>,
    ),
  );
  await session.connect();
  transport.emit({
    'type': 'hello',
    'pairs': [
      {
        'a': 'ja',
        'b': 'zh',
        'directions': [
          {'src': 'ja', 'tgt': 'zh'},
          {'src': 'zh', 'tgt': 'ja'},
        ],
        'converse': true,
      },
    ],
    'packs': <dynamic>[],
    'presetsInUse': [
      {'id': 'gemini', 'trainsOnData': true},
    ],
    'tts': 'system',
    'demo': false,
  });
  return (
    session: session,
    controller: controller,
    store: store,
    transport: transport,
  );
}

void main() {
  testWidgets('editable settings mask keys and offer Save only', (
    tester,
  ) async {
    final (:session, :controller, :store, transport: _) = await make();
    addTearDown(session.dispose);
    await controller.load();

    await tester.pumpWidget(
      wrap(SettingsScreen(session: session, controller: controller)),
    );
    await tester.pump();

    expect(find.byKey(const Key('secret-mask-llm')), findsOneWidget);
    expect(find.textContaining('••••abcd'), findsOneWidget);
    expect(find.textContaining('sk-llm-1234abcd'), findsNothing);
    expect(find.text('Test connection'), findsNothing);
    expect(find.text('Fetch models'), findsNothing);
    expect(find.byKey(const Key('save-settings')), findsOneWidget);
  });

  testWidgets(
    'a Provider card shows the last session error for that Provider',
    (tester) async {
      final (:session, :controller, :store, :transport) = await make();
      addTearDown(session.dispose);
      await controller.load();
      transport.emit({
        'type': 'error',
        'provider': 'asr',
        'kind': 'auth',
        'message': 'Deepgram rejected the key.',
      });
      await tester.pump();

      await tester.pumpWidget(
        wrap(SettingsScreen(session: session, controller: controller)),
      );
      await tester.pump();

      expect(find.byKey(const Key('provider-error')), findsOneWidget);
      expect(find.textContaining('Deepgram rejected the key.'), findsOneWidget);
    },
  );

  testWidgets('web settings are a read-only summary with no key or URL field', (
    tester,
  ) async {
    final (:session, :controller, :store, transport: _) = await make();
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(
        SettingsScreen(
          session: session,
          controller: controller,
          readOnly: true,
        ),
      ),
    );
    await tester.pump();

    expect(find.byKey(const Key('settings-read-only')), findsOneWidget);
    expect(find.text('gemini'), findsOneWidget);
    expect(find.textContaining('may train on your prompts'), findsOneWidget);
    expect(find.byType(EditableText), findsNothing);
    expect(find.text('Base URL'), findsNothing);
    expect(find.text('API key'), findsNothing);
    expect(find.byKey(const Key('save-settings')), findsNothing);
    expect(find.byKey(const Key('secret-mask-llm')), findsNothing);
  });

  testWidgets(
    'the real catalog lists Custom once and selecting it does not crash',
    (tester) async {
      final (:session, :controller, store: _, transport: _) = await make();
      addTearDown(session.dispose);
      await controller.load();

      await tester.pumpWidget(
        wrap(SettingsScreen(session: session, controller: controller)),
      );
      await tester.pump();

      await tester.tap(find.byKey(const Key('translator-preset')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Custom (Base URL + Model)'));
      await tester.pumpAndSettle();

      expect(tester.takeException(), isNull);
      expect(find.text('Custom (Base URL + Model)'), findsOneWidget);
    },
  );

  testWidgets('the System voice never claims the mock needs no key', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(800, 2000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final (:session, :controller, store: _, transport: _) = await make();
    addTearDown(session.dispose);
    await controller.load();

    await tester.pumpWidget(
      wrap(SettingsScreen(session: session, controller: controller)),
    );
    await tester.pump();

    expect(find.text('System voice'), findsOneWidget);
    expect(find.text('Mock needs no key.'), findsNothing);

    await tester.tap(find.byKey(const Key('tts-provider')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Mock').last);
    await tester.pumpAndSettle();
    expect(find.text('Mock needs no key.'), findsOneWidget);
  });
}
