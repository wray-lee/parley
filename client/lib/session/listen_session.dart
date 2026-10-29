import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../audio/audio_capture_api.dart';
import '../audio/tts_player_api.dart';
import '../model/protocol.dart';
import '../transport/transport.dart';
import 'tts_queue.dart';

enum SessionStatus { connecting, ready, listening, stopped, failed }

/// Listen Mode against the Server Host wire protocol: `hello`, Start/Stop,
/// the Demo sample, committed Segments and ordered TTS playback. One instance
/// per Session; the UI only reads this object and never builds a URL.
class ListenSession extends ChangeNotifier {
  ListenSession({
    required Transport transport,
    required AudioCapture audio,
    required TtsPlayer ttsPlayer,
    Uri? serverBase,
    Future<DemoClip?> Function()? loadDemoClip,
    http.Client? httpClient,
  }) : _transport = transport,
       _audio = audio,
       _serverBase = serverBase,
       _loadDemoClip = loadDemoClip,
       _http = httpClient ?? http.Client() {
    _tts = TtsQueue(
      player: ttsPlayer,
      onPlayback: _reportPlayback,
      onError: (error) {
        audioError = 'Audio playback failed: $error';
        _notify();
      },
    );
  }

  final Transport _transport;
  final AudioCapture _audio;
  final Uri? _serverBase;
  final Future<DemoClip?> Function()? _loadDemoClip;
  final http.Client _http;
  late final TtsQueue _tts;

  StreamSubscription<String>? _textSubscription;
  StreamSubscription<Uint8List>? _binarySubscription;

  final List<Segment> _segments = [];
  List<Segment> get segments => List<Segment>.unmodifiable(_segments);

  List<LanguagePair> pairs = const [];
  LanguagePair? selectedPair;
  bool demo = false;
  SessionStatus status = SessionStatus.connecting;
  String? error;
  String? audioError;
  bool _disposed = false;
  bool _starting = false;
  DemoClip? _sample;

  Future<void> connect() async {
    debugPrint('PARLEY connect');
    _textSubscription = _transport.text.listen(
      _handleText,
      onError: (Object _) => _fail('Connection lost.'),
    );
    _binarySubscription = _transport.binary.listen(
      _tts.chunk,
      onError: (Object _) => _fail('Connection lost.'),
    );
    try {
      await _transport.connect();
    } catch (_) {
      // The exception text can carry the WebSocket URI, token included
      // (web_socket_channel's IO backend does), so neither the banner nor the
      // log sees it. Auth and transport failures share one banner; ticket 16
      // adds the auth message.
      _fail('Connection failed.');
    }
  }

  void selectPair(LanguagePair? pair) {
    selectedPair = pair;
    _notify();
  }

  Future<void> start() async {
    if (_starting) return;
    if (status != SessionStatus.ready && status != SessionStatus.stopped) {
      return;
    }
    final pair = selectedPair;
    if (pair == null || pair.directions.isEmpty) {
      error = 'The server did not offer a language pair.';
      _notify();
      return;
    }
    _starting = true;
    try {
      DemoClip? sample;
      if (demo) {
        sample = _sample ?? await _fetchDemoClip();
        _sample = sample;
        if (sample == null) {
          error ??=
              'The Demo sample is unavailable from this server, '
              'so the keyless demo cannot start.';
          _notify();
          return;
        }
      }
      final direction = _startDirection(pair, sample);
      // A new Session restarts seg_id numbering from 1: drop the previous
      // Session's captions and audio, including any unfinished clip.
      _segments.clear();
      _tts.reset();
      _send({
        'type': 'start',
        'mode': 'listen',
        'src': direction.src,
        'tgt': direction.tgt,
      });
      if (sample != null) {
        _send({'type': 'sample', 'name': sample.id});
      }
      if (!await _audio.start(_sendBinary, demo: sample)) {
        _send({'type': 'stop'});
        error = 'Microphone unavailable: permission denied or no input device.';
        _notify();
        return;
      }
      status = SessionStatus.listening;
      error = null;
      _notify();
    } catch (error) {
      _fail('Could not start the session: $error');
    } finally {
      _starting = false;
    }
  }

  Future<void> stop() async {
    await _audio.stop();
    _send({'type': 'stop'});
    status = SessionStatus.stopped;
    _notify();
  }

  DirectionRef _startDirection(LanguagePair pair, DemoClip? sample) {
    if (sample != null) {
      for (final direction in pair.directions) {
        if (direction.src == sample.src && direction.tgt == sample.tgt) {
          return direction;
        }
      }
    }
    return pair.directions.first;
  }

  void _handleText(String raw) {
    final Map<String, dynamic> message;
    try {
      message = jsonDecode(raw) as Map<String, dynamic>;
    } catch (e) {
      error = 'Ignored a malformed server message: $e';
      _notify();
      return;
    }
    switch (message['type']) {
      case 'hello':
        _handleHello(message);
        break;
      case 'state':
        if (message['state'] == 'listening') {
          status = SessionStatus.listening;
          _notify();
        }
        break;
      case 'segment':
        _handleSegment(message);
        break;
      case 'tts_begin':
        _tts.begin(
          message['seg_id'] as int,
          message['format'] as String? ?? 'mp3',
        );
        break;
      case 'tts_end':
        _tts.end(message['seg_id'] as int);
        break;
      case 'error':
        _handleError(message);
        break;
      default:
        break;
    }
  }

  void _handleHello(Map<String, dynamic> message) {
    demo = message['demo'] == true;
    pairs = (message['pairs'] as List<dynamic>? ?? const [])
        .map((pair) => LanguagePair.fromJson(pair as Map<String, dynamic>))
        .toList(growable: false);
    selectedPair = pairs.isEmpty ? null : pairs.first;
    status = SessionStatus.ready;
    error = null;
    debugPrint('PARLEY hello demo=$demo pairs=${pairs.length}');
    _notify();
  }

  void _handleSegment(Map<String, dynamic> message) {
    final segId = message['seg_id'] as int;
    if (message['speak'] == false) _tts.silent(segId);
    // Committed output is never revised: a repeated seg_id is ignored.
    if (_segments.any((segment) => segment.segId == segId)) return;
    _segments.add(
      Segment(
        segId: segId,
        source: message['src'] as String? ?? '',
        translation: message['tgt'] as String? ?? '',
        demo: message['demo'] == true,
      ),
    );
    _segments.sort((a, b) => a.segId.compareTo(b.segId));
    debugPrint(
      'PARLEY segment ${message['seg_id']} src=${jsonEncode(message['src'])} '
      'tgt=${jsonEncode(message['tgt'])}',
    );
    _notify();
  }

  void _handleError(Map<String, dynamic> message) {
    final provider = message['provider'] as String?;
    final text = [
      if (provider != null) '[$provider]',
      message['message'] as String? ?? 'Unknown server error',
      if (message['fallback'] != null) '— ${message['fallback']}',
    ].join(' ');
    debugPrint('PARLEY server error $text');
    if (provider == 'tts') {
      audioError = text;
    } else {
      error = text;
    }
    _notify();
  }

  void _reportPlayback(int segId, String state) {
    debugPrint('PARLEY tts $segId $state');
    _send({'type': 'playback', 'seg_id': segId, 'state': state});
  }

  void _sendBinary(Uint8List frame) => _transport.sendBinary(frame);

  void _send(Map<String, Object?> message) =>
      _transport.sendText(jsonEncode(message));

  Future<DemoClip?> _fetchDemoClip() async {
    final loader = _loadDemoClip;
    if (loader != null) return loader();
    final base = _serverBase;
    if (base == null) return null;
    try {
      final response = await _http
          .get(base.resolve('/samples/placeholder.json'))
          .timeout(const Duration(seconds: 5));
      if (response.statusCode != 200) {
        throw http.ClientException('HTTP ${response.statusCode}');
      }
      return DemoClip.fromJson(
        jsonDecode(response.body) as Map<String, dynamic>,
      );
    } catch (error) {
      this.error = 'Demo sample could not be loaded: $error';
      return null;
    }
  }

  void _fail(String message) {
    debugPrint('PARLEY failed $message');
    error = message;
    status = SessionStatus.failed;
    _notify();
  }

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _textSubscription?.cancel();
    _binarySubscription?.cancel();
    _audio.stop();
    _tts.dispose();
    _transport.close();
    _http.close();
    super.dispose();
  }
}
