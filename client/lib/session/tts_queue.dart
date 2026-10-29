import 'dart:typed_data';

import '../audio/tts_player_api.dart';

/// Plays TTS chunks strictly in `seg_id` order even when a later Segment's
/// audio arrives first, and never lets one failed clip stop the queue.
class TtsQueue {
  TtsQueue({
    required TtsPlayer player,
    required void Function(int segId, String state) onPlayback,
    required void Function(Object error) onError,
  }) : _player = player,
       _onPlayback = onPlayback,
       _onError = onError;

  final TtsPlayer _player;
  final void Function(int segId, String state) _onPlayback;
  final void Function(Object error) _onError;

  final Map<int, _Clip> _clips = {};
  final Set<int> _silent = {};
  Future<void> _chain = Future<void>.value();
  int _next = 1;
  int? _active;

  /// Forgets all queued audio so a new Session's seg_id order starts at 1.
  void reset() {
    _clips.clear();
    _silent.clear();
    _active = null;
    _next = 1;
  }

  void begin(int segId, String format) {
    _active = segId;
    _clips.putIfAbsent(segId, () => _Clip(format));
  }

  void chunk(Uint8List data) => _clips[_active]?.chunks.add(data);

  void end(int segId) {
    final clip = _clips[segId];
    if (clip == null) return;
    clip.complete = true;
    if (_active == segId) _active = null;
    _schedule();
  }

  /// A Segment the server does not speak (Converse, other Turn): its seg_id
  /// will never receive audio, so the queue must not wait for it.
  void silent(int segId) {
    _silent.add(segId);
    _schedule();
  }

  void _schedule() {
    _chain = _chain.then((_) => _drain());
  }

  Future<void> _drain() async {
    while (true) {
      while (_silent.remove(_next)) {
        _next++;
      }
      final clip = _clips[_next];
      if (clip == null || !clip.complete) return;
      _clips.remove(_next);
      final segId = _next;
      _next++;
      _onPlayback(segId, 'start');
      try {
        // One source per chunk: independently encoded chunks must not be
        // concatenated into one buffer (spike S1 F1 truncates the tail).
        for (final data in clip.chunks) {
          await _player.play(clip.format, data);
        }
      } catch (error) {
        _onError(error);
      } finally {
        _onPlayback(segId, 'end');
      }
    }
  }

  Future<void> dispose() => _player.dispose();
}

class _Clip {
  _Clip(this.format);

  final String format;
  final List<Uint8List> chunks = [];
  bool complete = false;
}
