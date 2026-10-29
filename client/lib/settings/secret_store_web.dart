import 'secret_store.dart';

/// The web build has no key or server-URL field, so nothing is stored.
SecretStore createSecretStore() => MemorySecretStore();
