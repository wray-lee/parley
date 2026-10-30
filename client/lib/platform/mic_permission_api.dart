/// Why a live Session cannot capture the microphone, or that it can.
enum MicAccess {
  granted,

  /// The user (or the OS policy) refused microphone access.
  denied,

  /// The page is not a secure context, so the browser exposes no microphone.
  insecure,
}

/// The microphone permission seam. It is requested at the first live Start,
/// never at launch, and never in Demo Mode (which plays a bundled sample).
abstract interface class MicPermission {
  Future<MicAccess> request();

  /// Opens the OS app settings. False when the platform has no such screen
  /// (web), where the recovery card explains the browser instead.
  Future<bool> openSettings();
}

/// The seam tests and Demo-only hosts use: the microphone is always there.
class GrantedMicPermission implements MicPermission {
  const GrantedMicPermission();

  @override
  Future<MicAccess> request() async => MicAccess.granted;

  @override
  Future<bool> openSettings() async => false;
}
