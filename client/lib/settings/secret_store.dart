import 'secret_store_stub.dart'
    if (dart.library.io) 'secret_store_io.dart'
    if (dart.library.js_interop) 'secret_store_web.dart'
    as platform;

/// The seam under `flutter_secure_storage`: native builds use the platform
/// keystore; the web build keeps nothing, because web settings are read-only.
abstract interface class SecretStore {
  Future<String?> read(String key);
  Future<void> write(String key, String? value);
}

/// Where build/run time ends and the platform store begins.
SecretStore createSecretStore() => platform.createSecretStore();

/// Test and web store; never persisted anywhere.
class MemorySecretStore implements SecretStore {
  MemorySecretStore([Map<String, String>? initial]) : values = {...?initial};

  final Map<String, String> values;

  @override
  Future<String?> read(String key) async => values[key];

  @override
  Future<void> write(String key, String? value) async {
    if (value == null || value.isEmpty) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }
}
