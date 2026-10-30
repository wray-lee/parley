import 'device_host_api.dart';

/// The web build always talks to a Server Host; there is no embedded host.
DeviceHost? createDeviceHost() => null;
