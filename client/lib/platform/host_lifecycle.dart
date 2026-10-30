import 'dart:async';

import 'package:flutter/widgets.dart';

/// Foreground-only Device Host (S2): leaving the foreground stops the host,
/// returning starts a new one with a new port and token. The Android/iOS
/// plugin only owns the native host; this is the Flutter half of the choice.
class HostLifecycle with WidgetsBindingObserver {
  HostLifecycle({required this.onPause, required this.onResume});

  final Future<void> Function() onPause;
  final Future<void> Function() onResume;

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused) unawaited(onPause());
    if (state == AppLifecycleState.resumed) unawaited(onResume());
  }
}
