import 'mic_permission_api.dart';
import 'mic_permission_io.dart'
    if (dart.library.js_interop) 'mic_permission_web.dart'
    as impl;

export 'mic_permission_api.dart';

MicPermission createMicPermission() => impl.createMicPermission();
