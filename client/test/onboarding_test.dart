import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/onboarding_screen.dart';

import 'fakes.dart';

Widget wrap(Widget child) => MaterialApp(
  theme: parleyTheme(Brightness.dark),
  localizationsDelegates: parleyLocalizationDelegates,
  supportedLocales: parleySupportedLocales,
  home: child,
);

Map<String, dynamic> hello({bool demo = true}) => {
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
  'presetsInUse': <dynamic>[],
  'demo': demo,
  'tts': 'audio',
};

Future<ListenSession> sessionWith(
  WidgetTester tester, {
  bool demo = true,
}) async {
  final transport = FakeTransport();
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
    micPermission: FakeMicPermission(),
  );
  await session.connect();
  transport.emit(hello(demo: demo));
  await tester.pump();
  return session;
}

void main() {
  testWidgets(
    'a fresh install walks pair, try it, check audio in three steps',
    (tester) async {
      final session = await sessionWith(tester);
      var finish = <({bool startNow})>[];

      await tester.pumpWidget(
        wrap(
          OnboardingScreen(
            session: session,
            onFinish: ({required startNow}) => finish.add((startNow: startNow)),
            onSettings: () {},
            onUseKeys: () {},
          ),
        ),
      );

      expect(find.byKey(const Key('onboarding-pair-0')), findsOneWidget);
      expect(find.byKey(const Key('onboarding-dot-0')), findsOneWidget);
      await tester.tap(find.byKey(const Key('onboarding-pair-0')));
      await tester.tap(find.byKey(const Key('onboarding-next')));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('onboarding-try-demo')), findsOneWidget);
      await tester.tap(find.byKey(const Key('onboarding-try-demo')));
      await tester.tap(find.byKey(const Key('onboarding-next')));
      await tester.pumpAndSettle();

      expect(
        find.byKey(const Key('onboarding-output-earphones')),
        findsOneWidget,
      );
      expect(find.byKey(const Key('mic-primer')), findsOneWidget);
      await tester.tap(find.byKey(const Key('onboarding-finish')));
      await tester.pumpAndSettle();

      expect(finish.single.startNow, isTrue);
      expect(session.selectedPair, isNotNull);
      expect(session.loudspeakerOptIn, isFalse);
    },
  );

  testWidgets('the loudspeaker is an explicit opt-in', (tester) async {
    final session = await sessionWith(tester);
    await tester.pumpWidget(
      wrap(
        OnboardingScreen(
          session: session,
          onFinish: ({required startNow}) {},
          onSettings: () {},
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-output-loudspeaker')));
    await tester.tap(find.byKey(const Key('onboarding-finish')));
    await tester.pumpAndSettle();
    expect(session.loudspeakerOptIn, isTrue);
  });

  testWidgets('a server without Demo disables the Demo card', (tester) async {
    final session = await sessionWith(tester, demo: false);
    var started = true;
    await tester.pumpWidget(
      wrap(
        OnboardingScreen(
          session: session,
          onFinish: ({required startNow}) => started = startNow,
          onSettings: () {},
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-try-demo')));
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-finish')));
    await tester.pumpAndSettle();
    expect(started, isFalse);
  });

  testWidgets('Use my keys reaches Settings', (tester) async {
    final session = await sessionWith(tester);
    var opened = 0;
    await tester.pumpWidget(
      wrap(
        OnboardingScreen(
          session: session,
          onFinish: ({required startNow}) {},
          onSettings: () {},
          onUseKeys: () => opened++,
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-try-keys')));
    await tester.tap(find.byKey(const Key('onboarding-open-settings')));
    expect(opened, 1);
  });

  testWidgets('check audio plays a test sound through the system voice', (
    tester,
  ) async {
    final transport = FakeTransport();
    final engine = FakeSystemTtsEngine();
    final session = ListenSession(
      transport: transport,
      audio: FakeAudioCapture(),
      ttsPlayer: FakeTtsPlayer(),
      systemTtsEngine: engine,
      micPermission: FakeMicPermission(),
    );
    addTearDown(session.dispose);
    await session.connect();
    transport.emit(hello());
    await tester.pump();

    await tester.pumpWidget(
      wrap(
        OnboardingScreen(
          session: session,
          onFinish: ({required startNow}) {},
          onSettings: () {},
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('onboarding-test-sound')));
    await tester.pump();
    expect(
      engine.spoken,
      contains('Parley will speak your translations here.'),
    );
    engine.complete();
    await tester.pump();
    expect(find.byKey(const Key('onboarding-test-failed')), findsNothing);

    await tester.tap(find.byKey(const Key('onboarding-test-sound')));
    await tester.pump();
    engine.fail(StateError('no voice'), 1);
    await tester.pump();
    expect(find.byKey(const Key('onboarding-test-failed')), findsOneWidget);
  });

  testWidgets('onboarding never asks for the microphone', (tester) async {
    final session = await sessionWith(tester);
    await tester.pumpWidget(
      wrap(
        OnboardingScreen(
          session: session,
          onFinish: ({required startNow}) {},
          onSettings: () {},
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    expect(session.micRecovery, isNull);
  });
}
