import 'package:material_ui/material_ui.dart';

import 'app_config.dart';
import 'audio/audio_capture.dart';
import 'audio/tts_player.dart';
import 'platform/first_run_store.dart';
import 'platform/mic_permission.dart';
import 'session/listen_session.dart';
import 'settings/presets.dart';
import 'settings/secret_store.dart';
import 'settings/settings_controller.dart';
import 'transport/web_socket_transport.dart';
import 'ui/parley_app.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final config = AppConfig.fromEnvironment();
  final mic = createMicPermission();
  final firstRun = createFirstRunStore();
  final settings = SettingsController(
    store: createSecretStore(),
    presets: await PresetCatalog.load(),
  );
  await settings.load();
  runApp(
    ParleyApp(
      settings: settings,
      firstRun: firstRun,
      showOnboarding: !await firstRun.seen('onboarding'),
      createSession: () => ListenSession(
        transport: WebSocketTransport(config.wsUri),
        audio: createAudioCapture(),
        ttsPlayer: createTtsPlayer(),
        serverBase: config.baseUri,
        micPermission: mic,
      ),
    ),
  );
}
