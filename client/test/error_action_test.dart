import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/caption_stack.dart';

Widget wrap(Widget child) => MaterialApp(
  theme: parleyTheme(Brightness.dark),
  localizationsDelegates: parleyLocalizationDelegates,
  supportedLocales: parleySupportedLocales,
  home: Scaffold(body: child),
);

void main() {
  const nextActions = {
    'auth': 'Check the API key in Settings.',
    'quota': 'Wait for the quota to reset or top up, then try again.',
    'network': 'Check the network connection and the Base URL.',
    'model': 'Check the model name and Base URL in Settings.',
    'cors': 'The Provider blocks browser requests; connect through your Server Host.',
    'timeout': 'Try again, or choose a lighter model.',
  };

  for (final entry in nextActions.entries) {
    testWidgets('a ${entry.key} error offers its own next action', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          CaptionStack(
            segments: const <Segment>[],
            inlineError: 'Provider failed',
            inlineErrorKind: entry.key,
          ),
        ),
      );
      expect(find.text(entry.value), findsOneWidget);
    });
  }

  testWidgets('auth and model errors deep-link to Settings', (tester) async {
    var opened = 0;
    await tester.pumpWidget(
      wrap(
        CaptionStack(
          segments: const <Segment>[],
          inlineError: 'Bad key',
          inlineErrorKind: 'auth',
          onSettings: () => opened++,
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('error-open-settings')));
    expect(opened, 1);

    await tester.pumpWidget(
      wrap(
        CaptionStack(
          segments: const <Segment>[],
          inlineError: 'Bad model',
          inlineErrorKind: 'model',
          onSettings: () => opened++,
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('error-open-settings')));
    expect(opened, 2);
  });

  testWidgets('an unknown kind shows the message without a next action', (
    tester,
  ) async {
    await tester.pumpWidget(
      wrap(
        CaptionStack(
          segments: const <Segment>[],
          inlineError: 'Something happened',
          inlineErrorKind: 'weird',
        ),
      ),
    );
    expect(find.text('Something happened'), findsOneWidget);
    expect(find.byKey(const Key('error-next-action')), findsNothing);
  });

  testWidgets('an ASR failure shows the reconnecting chip', (tester) async {
    await tester.pumpWidget(
      wrap(const CaptionStack(segments: <Segment>[], asrReconnecting: true)),
    );
    expect(find.byKey(const Key('asr-reconnecting')), findsOneWidget);
  });
}
