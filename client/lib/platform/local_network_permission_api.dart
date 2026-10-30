/// Android 17 gates local-network traffic behind a runtime permission. The
/// request happens only when a LAN URL is saved; every other platform reports
/// that it has nothing to ask.
enum LocalNetworkAccess { notRequired, granted, denied }

abstract interface class LocalNetworkPermission {
  Future<LocalNetworkAccess> request();
}

class NotRequiredLocalNetworkPermission implements LocalNetworkPermission {
  const NotRequiredLocalNetworkPermission();

  @override
  Future<LocalNetworkAccess> request() async => LocalNetworkAccess.notRequired;
}
