import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'secret_store.dart';

SecretStore createSecretStore() => SecureSecretStore();

/// `flutter_secure_storage` on Android/iOS: keys live in the platform keystore
/// and never in a log, a web page or a plain preferences file.
class SecureSecretStore implements SecretStore {
  SecureSecretStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  final FlutterSecureStorage _storage;

  @override
  Future<String?> read(String key) => _storage.read(key: key);

  @override
  Future<void> write(String key, String? value) => value == null
      ? _storage.delete(key: key)
      : _storage.write(key: key, value: value);
}
