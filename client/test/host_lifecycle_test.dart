import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/platform/host_lifecycle.dart';

void main() {
  test('paused stops the host and resumed starts it again', () {
    var pauses = 0;
    var resumes = 0;
    final lifecycle = HostLifecycle(
      onPause: () async => pauses++,
      onResume: () async => resumes++,
    );

    lifecycle.didChangeAppLifecycleState(AppLifecycleState.inactive);
    lifecycle.didChangeAppLifecycleState(AppLifecycleState.hidden);
    expect(pauses, 0);
    expect(resumes, 0);

    lifecycle.didChangeAppLifecycleState(AppLifecycleState.paused);
    expect(pauses, 1);

    lifecycle.didChangeAppLifecycleState(AppLifecycleState.resumed);
    expect(resumes, 1);
  });
}
