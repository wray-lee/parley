import 'package:web/web.dart' as web;

import 'first_run_store_api.dart';

FirstRunStore createFirstRunStore() => const LocalFirstRunStore();

class LocalFirstRunStore implements FirstRunStore {
  const LocalFirstRunStore();

  static const _prefix = 'parley.firstRun.';

  // Storage can be blocked (private mode); showing onboarding again is the
  // safe fallback.
  @override
  Future<bool> seen(String flag) async {
    try {
      return web.window.localStorage.getItem('$_prefix$flag') == 'true';
    } catch (_) {
      return false;
    }
  }

  @override
  Future<void> markSeen(String flag) async {
    try {
      web.window.localStorage.setItem('$_prefix$flag', 'true');
    } catch (_) {}
  }
}
