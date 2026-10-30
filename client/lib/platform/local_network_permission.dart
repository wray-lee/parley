import 'local_network_permission_api.dart';
import 'local_network_permission_io.dart'
    if (dart.library.js_interop) 'local_network_permission_web.dart'
    as impl;

export 'local_network_permission_api.dart';

LocalNetworkPermission createLocalNetworkPermission() =>
    impl.createLocalNetworkPermission();
