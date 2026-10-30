import 'package:flutter/services.dart';

import 'local_network_permission_api.dart';

LocalNetworkPermission createLocalNetworkPermission() =>
    const PlatformLocalNetworkPermission();

/// The Kotlin side asks `ACCESS_LOCAL_NETWORK` and answers granted/denied;
/// every non-Android host has no such channel and needs no permission.
class PlatformLocalNetworkPermission implements LocalNetworkPermission {
  const PlatformLocalNetworkPermission();

  static const _channel = MethodChannel('app.parley/local_network');

  @override
  Future<LocalNetworkAccess> request() async {
    try {
      final value = await _channel.invokeMethod<String>('request');
      return switch (value) {
        'granted' => LocalNetworkAccess.granted,
        'denied' => LocalNetworkAccess.denied,
        _ => LocalNetworkAccess.notRequired,
      };
    } on MissingPluginException {
      // iOS and desktop have no such channel and no such permission.
      return LocalNetworkAccess.notRequired;
    } catch (_) {
      // A channel that exists but failed is treated as a denial, so the user
      // sees the recovery message instead of a silent later network failure.
      return LocalNetworkAccess.denied;
    }
  }
}
