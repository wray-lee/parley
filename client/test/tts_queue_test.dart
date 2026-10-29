import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/audio/tts_player_api.dart';
import 'package:parley/session/tts_queue.dart';

/// A player whose chunks settle only when the test says so; `stop` deliberately
/// leaves them pending, like the web `<audio>` player.
class ControlledPlayer implements TtsPlayer {
  final List<String> played = [];
  final List<Completer<void>> pending = [];
  int stops = 0;
  bool disposed = false;

  @override
  Future<void> play(String format, Uint8List data) {
    played.add(utf8.decode(data));
    final completer = Completer<void>();
    pending.add(completer);
    return completer.future;
  }

  void complete([int index = 0]) {
    if (!pending[index].isCompleted) pending[index].complete();
  }

  void fail(Object error, [int index = 0]) {
    if (!pending[index].isCompleted) pending[index].completeError(error);
  }

  @override
  Future<void> stop() async {
    stops++;
  }

  @override
  Future<void> dispose() async {
    disposed = true;
  }
}

Uint8List bytes(String text) => Uint8List.fromList(utf8.encode(text));

void main() {
  late ControlledPlayer player;
  late List<String> reports;
  late List<Object> errors;
  late TtsQueue queue;

  setUp(() {
    player = ControlledPlayer();
    reports = [];
    errors = [];
    queue = TtsQueue(
      player: player,
      onPlayback: (segId, state) => reports.add('$segId:$state'),
      onError: errors.add,
    );
  });

  test('plays chunks in seg_id order and reports one start/end pair', () async {
    queue.begin(2, 'mp3');
    queue.chunk(bytes('two'));
    queue.end(2);
    queue.begin(1, 'mp3');
    queue.chunk(bytes('one'));
    queue.end(1);
    await pumpEventQueue();

    expect(player.played, ['one']);
    expect(reports, ['1:start']);

    player.complete(0);
    await pumpEventQueue();
    expect(player.played, ['one', 'two']);
    expect(reports, ['1:start', '1:end', '2:start']);

    player.complete(1);
    await pumpEventQueue();
    expect(reports, ['1:start', '1:end', '2:start', '2:end']);
  });

  test('skips Segments the server does not speak', () async {
    queue.silent(1);
    queue.begin(2, 'mp3');
    queue.chunk(bytes('two'));
    queue.end(2);
    await pumpEventQueue();

    expect(player.played, ['two']);
    expect(reports, ['2:start']);
  });

  test(
    'a failed chunk ends that Segment and keeps the next in order',
    () async {
      queue.begin(1, 'mp3');
      queue.chunk(bytes('one'));
      queue.end(1);
      queue.begin(2, 'mp3');
      queue.chunk(bytes('two'));
      queue.end(2);
      await pumpEventQueue();

      player.fail(StateError('route changed'));
      await pumpEventQueue();

      expect(errors.single, isA<StateError>());
      expect(reports, ['1:start', '1:end', '2:start']);
      expect(player.played, ['one', 'two']);
    },
  );

  test('stop ends the in-flight Segment and lets a new Session play seg 1 '
      'without waiting for a hung player', () async {
    queue.begin(1, 'mp3');
    queue.chunk(bytes('old'));
    queue.end(1);
    await pumpEventQueue();
    expect(player.played, ['old']);

    await queue.stop();
    expect(reports, ['1:start', '1:end']);
    expect(player.stops, greaterThan(0));

    queue.begin(1, 'mp3');
    queue.chunk(bytes('new'));
    queue.end(1);
    await pumpEventQueue();
    expect(player.played, ['old', 'new']);
    expect(reports, ['1:start', '1:end', '1:start']);

    // The hung old chunk settling late must not close the new Segment.
    player.complete(0);
    await pumpEventQueue();
    expect(reports, ['1:start', '1:end', '1:start']);

    player.complete(1);
    await pumpEventQueue();
    expect(reports, ['1:start', '1:end', '1:start', '1:end']);
  });

  test(
    'reset drops the old Session without reporting its playback end',
    () async {
      queue.begin(1, 'mp3');
      queue.chunk(bytes('old'));
      queue.end(1);
      await pumpEventQueue();

      queue.reset();
      queue.begin(1, 'mp3');
      queue.chunk(bytes('new'));
      queue.end(1);
      await pumpEventQueue();

      expect(player.played, ['old', 'new']);
      expect(reports, ['1:start', '1:start']);

      player.complete(1);
      await pumpEventQueue();
      expect(reports, ['1:start', '1:start', '1:end']);
    },
  );

  test(
    'an interruption between chunks of one Segment still ends it once',
    () async {
      queue.begin(1, 'mp3');
      queue.chunk(bytes('a'));
      queue.chunk(bytes('b'));
      queue.end(1);
      await pumpEventQueue();
      expect(player.played, ['a']);

      await queue.stop();
      expect(reports, ['1:start', '1:end']);
      expect(player.played, ['a']);

      player.complete(0);
      await pumpEventQueue();
      expect(player.played, ['a']);
      expect(reports, ['1:start', '1:end']);
    },
  );
}
