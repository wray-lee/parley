import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// The manifest is the only place Android permissions are declared; a missing
/// ACCESS_LOCAL_NETWORK makes the runtime request fail on Android 17.
void main() {
  test('the Android manifest declares audio, internet and local network', () {
    final manifest = File('android/app/src/main/AndroidManifest.xml')
        .readAsStringSync();
    for (final permission in [
      'android.permission.INTERNET',
      'android.permission.RECORD_AUDIO',
      'android.permission.ACCESS_LOCAL_NETWORK',
    ]) {
      expect(
        manifest,
        contains('android:name="$permission"'),
        reason: '$permission must be declared',
      );
    }
  });

  test('iOS explains the microphone and local network in Info.plist', () {
    final plist = File('ios/Runner/Info.plist').readAsStringSync();
    expect(plist, contains('NSMicrophoneUsageDescription'));
    expect(plist, contains('NSLocalNetworkUsageDescription'));
  });
}
