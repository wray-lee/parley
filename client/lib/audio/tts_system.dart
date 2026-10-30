import 'dart:async';
import 'dart:collection';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter_tts/flutter_tts.dart';

/// The platform system speech engine, reduced to what [SystemTtsSpeaker]
/// needs. `speak` completes when the utterance ends — completion, cancel or
/// error — and the speaker's watchdog covers engines that never report.
abstract interface class SystemTtsEngine {
  Future<void> speak(String text, {String? language});
  Future<void> stop();
  Future<void> dispose();
}

/// flutter_tts 4.2.5, settled by its completion/cancel/error handlers rather
/// than the `speak()` future: iOS never resolves that future on cancel and the
/// web plugin leaves it pending on error (the documented hang).
class FlutterTtsEngine implements SystemTtsEngine {
  FlutterTtsEngine([FlutterTts? tts]) : _tts = tts ?? FlutterTts() {
    _tts.setCompletionHandler(_finish);
    _tts.setCancelHandler(_finish);
    _tts.setErrorHandler(_error);
  }

  final FlutterTts _tts;
  Completer<void>? _utterance;

  void _finish() {
    final utterance = _utterance;
    _utterance = null;
    if (utterance != null && !utterance.isCompleted) utterance.complete();
  }

  void _error(dynamic message) {
    final utterance = _utterance;
    _utterance = null;
    if (utterance != null && !utterance.isCompleted) {
      utterance.completeError(StateError('system TTS failed: $message'));
    }
  }

  @override
  Future<void> speak(String text, {String? language}) async {
    _finish(); // never overlap a stale utterance with the next one
    if (language != null && language.isNotEmpty) {
      // Unknown tags fall back to the engine's current voice.
      await _tts.setLanguage(language);
    }
    final done = Completer<void>();
    _utterance = done;
    // The handlers above own the utterance's lifetime. 0 means the engine
    // refused the utterance (iOS resume failure, Android queue flush) and
    // false is a defensive guard; a hung engine is the watchdog's job.
    final result = await _tts.speak(text);
    if (result == false || result == 0) {
      _utterance = null;
      throw StateError('system TTS is unavailable');
    }
    return done.future;
  }

  @override
  Future<void> stop() async {
    await _tts.stop();
    _finish();
  }

  @override
  Future<void> dispose() => stop();
}

/// The core releases Gating at `estimateSpeechMs(text) + 1.5 s` when no
/// playback end arrives; the client waits the same time before unblocking its
/// queue. String.length counts UTF-16 code units, like the core.
Duration systemSpeechWatchdog(String text) =>
    Duration(milliseconds: math.min(30000, 1500 + text.length * 180) + 1500);

/// Speaks `speak: true` Segments with the platform engine, one at a time, and
/// reports playback start/end for core-owned Gating.
class SystemTtsSpeaker {
  SystemTtsSpeaker({
    required SystemTtsEngine engine,
    required this.onPlayback,
    required this.onError,
    this.watchdog = systemSpeechWatchdog,
  }) : _engine = engine;

  final SystemTtsEngine _engine;
  final void Function(int segId, String state) onPlayback;
  final void Function(Object error) onError;
  final Duration Function(String text) watchdog;

  final Queue<_Utterance> _pending = Queue<_Utterance>();
  _Utterance? _current;
  Future<void>? _drain;

  void speak(int segId, String text, {String? language}) {
    if (text.trim().isEmpty) return;
    _pending.add(_Utterance(segId, text, language));
    _drain ??= _run();
  }

  /// Speaks outside a Session (the onboarding audio check) without reporting
  /// a seg_id the core never issued.
  Future<void> say(String text, {String? language}) =>
      _engine.speak(text, language: language);

  Future<void> _run() async {
    try {
      while (_pending.isNotEmpty) {
        final utterance = _pending.removeFirst();
        _current = utterance;
        onPlayback(utterance.segId, 'start');
        _start(utterance);
        await utterance.done.future;
        // A reset belongs to a new Session: its end must not reach the core
        // as the new Session's seg_id.
        if (!utterance.detached) onPlayback(utterance.segId, 'end');
        _current = null;
      }
    } finally {
      _current = null;
      _drain = null;
    }
  }

  void _start(_Utterance utterance) {
    utterance.timer = Timer(watchdog(utterance.text), () {
      debugPrint('PARLEY tts watchdog ${utterance.segId}');
      // No completion, cancel or error arrived: the engine is hung or
      // missing. Silence is not a valid outcome, so raise the chip.
      if (!utterance.detached && !utterance.done.isCompleted) {
        onError(StateError('system TTS did not report completion'));
      }
      _settle(utterance);
    });
    _engine
        .speak(utterance.text, language: utterance.language)
        .then(
          (_) => _settle(utterance),
          onError: (Object error) => _fail(utterance, error),
        );
  }

  void _settle(_Utterance utterance) {
    utterance.timer?.cancel();
    if (!utterance.done.isCompleted) utterance.done.complete();
  }

  void _fail(_Utterance utterance, Object error) {
    onError(error);
    _settle(utterance);
  }

  /// Drops queued and in-flight speech without waiting on the engine: a new
  /// Session's seg_id order must not queue behind the previous one.
  void reset() {
    _pending.clear();
    final current = _current;
    if (current != null) {
      current.detached = true;
      _settle(current);
    }
    unawaited(_stopEngine());
  }

  /// Cancels speech, ends the in-flight utterance and waits for its playback
  /// end report so Gating is released before the Session stops.
  Future<void> stop() async {
    _pending.clear();
    final current = _current;
    if (current != null) _settle(current);
    final drain = _drain;
    await _stopEngine();
    await drain;
  }

  Future<void> dispose() async {
    await stop();
    await _engine.dispose();
  }

  Future<void> _stopEngine() async {
    try {
      await _engine.stop();
    } catch (error) {
      // Engine stop failures are not speech failures; the watchdog already
      // ends the utterance.
      debugPrint('PARLEY tts stop $error');
    }
  }
}

class _Utterance {
  _Utterance(this.segId, this.text, this.language);

  final int segId;
  final String text;
  final String? language;
  final Completer<void> done = Completer<void>();
  Timer? timer;
  bool detached = false;
}
