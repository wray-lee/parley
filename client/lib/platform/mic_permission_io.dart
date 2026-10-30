import 'package:permission_handler/permission_handler.dart';

import 'mic_permission_api.dart';

MicPermission createMicPermission() => const HandlerMicPermission();

class HandlerMicPermission implements MicPermission {
  const HandlerMicPermission();

  @override
  Future<MicAccess> request() async {
    try {
      final status = await Permission.microphone.request();
      return status.isGranted || status.isLimited
          ? MicAccess.granted
          : MicAccess.denied;
    } catch (_) {
      // A platform without a microphone permission handler is a denial, not
      // a crash on Start.
      return MicAccess.denied;
    }
  }

  @override
  Future<bool> openSettings() async {
    try {
      return await openAppSettings();
    } catch (_) {
      return false;
    }
  }
}
