import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'first_run_store_api.dart';

FirstRunStore createFirstRunStore() => SecureFirstRunStore();

/// `flutter_secure_storage` is already the only native store; a UI flag in it
/// costs no new dependency and survives reinstall-free upgrades.
class SecureFirstRunStore implements FirstRunStore {
  SecureFirstRunStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  final FlutterSecureStorage _storage;

  static const _prefix = 'parley.firstRun.';

  // A flag that cannot be read is treated as unseen; failing to write it at
  // most shows onboarding again. Neither may take the app down.
  @override
  Future<bool> seen(String flag) async {
    try {
      return await _storage.read(key: '$_prefix$flag') == 'true';
    } catch (_) {
      return false;
    }
  }

  @override
  Future<void> markSeen(String flag) async {
    try {
      await _storage.write(key: '$_prefix$flag', value: 'true');
    } catch (_) {}
  }
}
