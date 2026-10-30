import 'package:wakelock_plus/wakelock_plus.dart';

/// Keeps the screen awake for the whole Listen Session. Injected into
/// [ListenScreen] so widget tests never touch the platform channel.
Future<void> keepAwake(bool on) async {
  if (on) {
    await WakelockPlus.enable();
  } else {
    await WakelockPlus.disable();
  }
}
