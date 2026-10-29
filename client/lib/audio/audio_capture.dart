import 'audio_capture_api.dart';
import 'audio_capture_io.dart'
    if (dart.library.js_interop) 'audio_capture_web.dart'
    as impl;

export 'audio_capture_api.dart';

AudioCapture createAudioCapture() => impl.createAudioCapture();
