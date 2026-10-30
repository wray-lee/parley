import 'package:permission_handler/permission_handler.dart';
import 'package:web/web.dart' as web;

import 'mic_permission_api.dart';

MicPermission createMicPermission() => const BrowserMicPermission();

/// The microphone exists only in a secure context: HTTPS, or localhost. The
/// check runs before asking so an http:// LAN page gets the HTTPS explanation
/// instead of a bare browser denial.
class BrowserMicPermission implements MicPermission {
  const BrowserMicPermission();

  @override
  Future<MicAccess> request() async {
    if (!web.window.isSecureContext) return MicAccess.insecure;
    try {
      final status = await Permission.microphone.request();
      return status.isGranted ? MicAccess.granted : MicAccess.denied;
    } catch (_) {
      return MicAccess.denied;
    }
  }

  @override
  Future<bool> openSettings() async => false;
}
