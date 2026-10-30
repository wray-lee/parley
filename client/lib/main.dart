import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/semantics.dart';
import 'package:material_ui/material_ui.dart';

import 'app_config.dart';
import 'audio/audio_capture.dart';
import 'audio/tts_player.dart';
import 'platform/device_host.dart';
import 'platform/first_run_store.dart';
import 'platform/host_lifecycle.dart';
import 'platform/mic_permission.dart';
import 'session/listen_session.dart';
import 'settings/host_environment.dart';
import 'settings/presets.dart';
import 'settings/secret_store.dart';
import 'settings/settings_controller.dart';
import 'transport/device_host_transport.dart';
import 'transport/transport.dart';
import 'transport/web_socket_transport.dart';
import 'ui/parley_app.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  if (kIsWeb) {
    // Flutter Web semantics are opt-in; without the handle VoiceOver and
    // TalkBack have nothing to read.
    SemanticsBinding.instance.ensureSemantics();
  }
  final config = AppConfig.fromEnvironment();
  final mic = createMicPermission();
  final firstRun = createFirstRunStore();
  final presets = await PresetCatalog.load();

  late final Transport transport;
  final settings = SettingsController(
    store: createSecretStore(),
    presets: presets,
    restartHost: () async {
      final current = transport;
      if (current is DeviceHostTransport) await current.restart();
    },
  );
  await settings.load();

  // Native builds with no PARLEY_URL run the core in-process; web and an
  // explicit PARLEY_URL keep the Server Host transport.
  final embedded = config.hasServer ? null : createDeviceHost();
  final hostTransport = embedded == null
      ? null
      : DeviceHostTransport(
          host: embedded,
          environment: () => hostEnvironment(settings.values, presets),
        );
  Transport openServer(String token) =>
      WebSocketTransport(config.wsUriFor(token));
  transport = hostTransport ?? openServer(config.token);
  if (hostTransport != null) {
    WidgetsBinding.instance.addObserver(
      HostLifecycle(
        onPause: hostTransport.suspend,
        onResume: hostTransport.resume,
      ),
    );
  }

  runApp(
    ParleyApp(
      settings: settings,
      firstRun: firstRun,
      showOnboarding: !await firstRun.seen('onboarding'),
      createSession: () => ListenSession(
        transport: transport,
        reopenTransport: hostTransport == null ? openServer : null,
        audio: createAudioCapture(),
        ttsPlayer: createTtsPlayer(),
        serverBase: config.hasServer ? config.baseUri : null,
        serverBaseProvider: hostTransport == null
            ? null
            : () => hostTransport.serverBase,
        micPermission: mic,
      ),
    ),
  );
}
