import 'package:material_ui/material_ui.dart';

import 'app_config.dart';
import 'audio/audio_capture.dart';
import 'audio/tts_player.dart';
import 'session/listen_session.dart';
import 'transport/web_socket_transport.dart';
import 'ui/parley_app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  final config = AppConfig.fromEnvironment();
  runApp(
    ParleyApp(
      createSession: () => ListenSession(
        transport: WebSocketTransport(config.wsUri),
        audio: createAudioCapture(),
        ttsPlayer: createTtsPlayer(),
        serverBase: config.baseUri,
      ),
    ),
  );
}
