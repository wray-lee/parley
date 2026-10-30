import 'dart:convert';

import 'package:flutter/services.dart';

import 'device_host_api.dart';

DeviceHost? createDeviceHost() => const PlatformDeviceHost();

/// One channel for the embedded host, matching `app.parley/core` in
/// MainActivity.kt and AppDelegate.swift.
class PlatformDeviceHost implements DeviceHost {
  const PlatformDeviceHost();

  static const channel = MethodChannel('app.parley/core');

  @override
  Future<HostEndpoint> start(Map<String, String> environment) async {
    try {
      final raw = await channel.invokeMethod<String>(
        'start',
        jsonEncode(environment),
      );
      if (raw == null) {
        throw const DeviceHostException(
          'The on-device host returned no endpoint.',
        );
      }
      return parseStartResult(raw);
    } on MissingPluginException {
      throw const DeviceHostException(
        'The on-device host is not available in this build.',
      );
    } on PlatformException catch (error) {
      // The native side only sends Go's validation message; redact anyway so a
      // key pasted into a wrong field can never surface.
      throw DeviceHostException(
        redactSecrets(
          error.message ?? 'The on-device host could not start.',
          environment.values,
        ),
      );
    }
  }

  @override
  Future<void> stop() async {
    try {
      await channel.invokeMethod<void>('stop');
    } on MissingPluginException {
      // Web and tests have no native host.
    } on PlatformException {
      // Stop is best effort; a failed stop must not crash Settings or dispose.
    }
  }
}
