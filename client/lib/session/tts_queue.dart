import 'dart:async';
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
  _Cancel? _cancel;

  /// Forgets all queued audio so a new Session's seg_id order starts at 1.
  /// The cancelled clip's end is not reported: it belongs to the old Session.
  void reset() {
    _clips.clear();
    _silent.clear();
    _active = null;
    _next = 1;
    _cancelPlayback(reportEnd: false);
  }

  /// Stops playback, reporting the in-flight clip's end so Gating releases,
  /// then drops everything queued.
  Future<void> stop() async {
    _clips.clear();
    _silent.clear();
    _active = null;
    _next = 1;
    _cancelPlayback(reportEnd: true);
    await _safePlayerStop();
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
      final cancel = _Cancel();
      _cancel = cancel;
      _onPlayback(segId, 'start');
      try {
        // One source per chunk: independently encoded chunks must not be
        // concatenated into one buffer (spike S1 F1 truncates the tail).
        for (final data in clip.chunks) {
          final playing = _player.play(clip.format, data);
          // A player that lost the route or was interrupted may never settle;
          // stop/reset must still release the queue.
          await Future.any([playing, cancel.done.future]);
          if (cancel.done.isCompleted) break;
        }
      } catch (error) {
        _onError(error);
      } finally {
        if (identical(_cancel, cancel)) _cancel = null;
        if (!cancel.done.isCompleted || cancel.reportEnd) {
          _onPlayback(segId, 'end');
        }
      }
      if (cancel.done.isCompleted) return;
    }
  }

  void _cancelPlayback({required bool reportEnd}) {
    final cancel = _cancel;
    _cancel = null;
    if (cancel != null && !cancel.done.isCompleted) {
      cancel.reportEnd = reportEnd;
      cancel.done.complete();
    }
    unawaited(_safePlayerStop());
  }

  Future<void> _safePlayerStop() async {
    try {
      await _player.stop();
    } catch (_) {
      // A player that cannot stop must not keep the queue alive.
    }
  }

  Future<void> dispose() async {
    await stop();
    await _player.dispose();
  }
}

class _Clip {
  _Clip(this.format);

  final String format;
  final List<Uint8List> chunks = [];
  bool complete = false;
}

class _Cancel {
  final Completer<void> done = Completer<void>();

  /// True for a Session stop (report playback end), false for a reset.
  bool reportEnd = true;
}
