import 'first_run_store_api.dart';
import 'first_run_store_io.dart'
    if (dart.library.js_interop) 'first_run_store_web.dart'
    as impl;

export 'first_run_store_api.dart';

FirstRunStore createFirstRunStore() => impl.createFirstRunStore();
