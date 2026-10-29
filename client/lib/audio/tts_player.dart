import 'tts_player_api.dart';
import 'tts_player_native.dart'
    if (dart.library.js_interop) 'tts_player_web.dart'
    as impl;

export 'tts_player_api.dart';

TtsPlayer createTtsPlayer() => impl.createTtsPlayer();
