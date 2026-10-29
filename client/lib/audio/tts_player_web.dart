import 'dart:async';
import 'dart:js_interop';
import 'dart:typed_data';

import 'package:web/web.dart' as web;

import 'tts_player_api.dart';

/// Web TTS playback through one HTML `<audio>` element per chunk; chunks are
/// independently decodable, so no Web Audio graph is needed.
class HtmlAudioTtsPlayer implements TtsPlayer {
  web.HTMLAudioElement? _element;
  String? _url;

  @override
  Future<void> play(String format, Uint8List data) async {
    await _release();
    final blob = web.Blob(
      <JSAny>[data.toJS].toJS,
      web.BlobPropertyBag(type: format == 'wav' ? 'audio/wav' : 'audio/mpeg'),
    );
    final url = web.URL.createObjectURL(blob);
    final element = web.document.createElement('audio') as web.HTMLAudioElement;
    _element = element;
    _url = url;
    final done = Completer<void>();
    element.onended = ((web.Event _) {
      if (!done.isCompleted) done.complete();
    }).toJS;
    element.onerror = ((web.Event _) {
      if (!done.isCompleted) {
        done.completeError(StateError('audio element failed'));
      }
    }).toJS;
    element.src = url;
    try {
      await element.play().toDart;
      await done.future.timeout(const Duration(seconds: 30));
    } finally {
      element.pause();
      // An interrupted clip can settle after the next one started; never
      // release the newer element.
      if (identical(_element, element)) {
        await _release();
      } else {
        web.URL.revokeObjectURL(url);
      }
    }
  }

  @override
  Future<void> stop() async {
    _element?.pause();
  }

  @override
  Future<void> dispose() => _release();

  Future<void> _release() async {
    _element?.pause();
    _element = null;
    final url = _url;
    _url = null;
    if (url != null) web.URL.revokeObjectURL(url);
  }
}

TtsPlayer createTtsPlayer() => HtmlAudioTtsPlayer();
