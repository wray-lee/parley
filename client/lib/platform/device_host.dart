import 'device_host_api.dart';
import 'device_host_io.dart'
    if (dart.library.js_interop) 'device_host_web.dart'
    as platform;

export 'device_host_api.dart';

/// Null on web: that build has no embedded host and uses a Server Host URL.
DeviceHost? createDeviceHost() => platform.createDeviceHost();
