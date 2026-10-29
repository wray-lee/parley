import 'secret_store.dart';

/// Unsupported platforms (desktop test hosts) keep settings in memory.
SecretStore createSecretStore() => MemorySecretStore();
