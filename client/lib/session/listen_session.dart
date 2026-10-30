import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../audio/audio_capture_api.dart';
import '../audio/tts_player_api.dart';
import '../audio/tts_system.dart';
import '../model/mode.dart';
import '../model/protocol.dart';
import '../platform/device_host_api.dart';
import '../platform/mic_permission_api.dart';
import '../transport/transport.dart';
import 'tts_queue.dart';

enum SessionStatus { connecting, ready, listening, stopped, failed }

/// One Session against the Server Host wire protocol, Listen or Converse:
/// `hello`, Start/Stop, the Demo sample, committed Segments, Turns and gated
/// TTS playback. One instance per Session; the UI only reads this object and
/// never builds a URL.
class ListenSession extends ChangeNotifier {
  ListenSession({
    required Transport transport,
    required AudioCapture audio,
    required TtsPlayer ttsPlayer,
    SystemTtsEngine? systemTtsEngine,
    Uri? serverBase,
    Uri? Function()? serverBaseProvider,
    Future<DemoClip?> Function()? loadDemoClip,
    Future<List<SampleRef>> Function()? loadSamples,
    http.Client? httpClient,
    MicPermission? micPermission,
  }) : _transport = transport,
       _audio = audio,
       _systemTtsEngine = systemTtsEngine,
       _serverBase = serverBase,
       _serverBaseProvider = serverBaseProvider,
       _loadDemoClip = loadDemoClip,
       _loadSamples = loadSamples,
       _http = httpClient ?? http.Client(),
       _mic = micPermission ?? const GrantedMicPermission() {
    _tts = TtsQueue(
      player: ttsPlayer,
      onPlayback: _reportPlayback,
      onError: _ttsFailed,
    );
  }

  final Transport _transport;
  final AudioCapture _audio;
  final SystemTtsEngine? _systemTtsEngine;
  final Uri? _serverBase;

  /// The Device Host's port and token change on every launch; a provider lets
  /// the Session read the current `/samples` origin instead of a stale URL.
  final Uri? Function()? _serverBaseProvider;
  final Future<DemoClip?> Function()? _loadDemoClip;
  final Future<List<SampleRef>> Function()? _loadSamples;
  final http.Client _http;
  final MicPermission _mic;
  late final TtsQueue _tts;
  SystemTtsSpeaker? _systemTts;

  /// Built on first use: audio Sessions never touch the OS speech engine.
  SystemTtsSpeaker get _systemSpeaker => _systemTts ??= SystemTtsSpeaker(
    engine: _systemTtsEngine ?? FlutterTtsEngine(),
    onPlayback: _reportPlayback,
    onError: _ttsFailed,
  );

  StreamSubscription<String>? _textSubscription;
  StreamSubscription<Uint8List>? _binarySubscription;

  final List<Segment> _segments = [];
  List<Segment> get segments => List<Segment>.unmodifiable(_segments);

  List<LanguagePair> pairs = const [];
  List<PackRef> packs = const [];
  List<PresetRef> presetsInUse = const [];
  List<SampleRef> samples = const [];
  LanguagePair? selectedPair;

  /// The Direction this Session receives audio for; set by the Start screen.
  DirectionRef? selectedDirection;

  /// Domain Packs selected on the Start screen, sent with `start`.
  Set<String> selectedPacks = {};

  /// The bundled sample to stream in Demo Mode; null means the server's
  /// default (`placeholder`), which the host advertises by convention.
  String? selectedSampleId;

  /// The last error each Provider reported, keyed by `asr`, `translator` or
  /// `tts`; the Settings cards show the last session's error.
  final Map<String, String> providerErrors = {};

  /// `hello.tts`: `system` (the client speaks) or `audio` (the host streams).
  String tts = '';
  bool demo = false;

  SessionMode selectedMode = SessionMode.listen;

  /// Converse only: `me` or `them` from the core's `turn` event.
  String? turn;
  bool turnAuto = false;

  /// Auto Turn capability from `hello.autoTurn`. False means Manual with the
  /// visible note; v0.1 ships Manual because no Provider passed S3.
  bool autoTurnAvailable = false;

  /// The user's opt-in Auto/Manual switch (S3 no-go: never a default).
  bool autoTurnEnabled = false;

  /// The Provider tag of the last decided Segment, for the Auto chip.
  String? autoLanguage;

  /// Increments on an undecided Auto Turn; the pads pulse.
  int padCue = 0;

  /// Increments on every Turn change after the first (`turnCueWho` names the
  /// new speaker). The UI cues motion and a haptic off it.
  int turnCue = 0;
  String? turnCueWho;

  /// Converse only: the core withholds the microphone while my spoken Segment
  /// plays plus a 500 ms tail (ADR-0002), so the screen can say so.
  bool gated = false;
  Timer? _gateTail;

  DirectionRef? _meDirection;
  DirectionRef? _themDirection;

  bool get isConverse => selectedMode == SessionMode.converse;

  /// The Direction my own speech takes (Converse); null outside it.
  DirectionRef? get meDirection => _meDirection;

  /// The Direction their speech takes, the reverse of [meDirection].
  DirectionRef? get themDirection => _themDirection;

  void selectMode(SessionMode mode) {
    selectedMode = mode;
    _notify();
  }

  /// A pad tap in Converse: the core selects the Direction that receives audio.
  /// In Auto it is a Turn Override even on the current pad (it cancels a
  /// pending switch) and it interrupts my playback.
  void takeTurn(String who) {
    if (!isConverse || !_isSpeaker(who)) return;
    if (turn == who && !autoTurnEnabled) return;
    unawaited(_tts.stop());
    _send({'type': 'turn', 'who': who});
  }

  /// Push-to-talk down: the Turn follows the pad, then automatic Cuts suspend.
  void beginPushToTalk(String who) {
    if (!isConverse || !_isSpeaker(who)) return;
    if (turn != who || autoTurnEnabled) {
      unawaited(_tts.stop());
      _send({'type': 'turn', 'who': who});
    }
    _send({'type': 'ptt', 'state': 'down'});
  }

  /// Push-to-talk up: the core Cuts the phrase on release.
  void endPushToTalk() {
    if (!isConverse) return;
    _send({'type': 'ptt', 'state': 'up'});
  }

  /// The Auto/Manual switch. Ignored when the Provider/pair did not pass the
  /// capability gate, so the UI can never force Auto on a Manual Session.
  void setAutoTurn(bool enabled) {
    if (!isConverse || !autoTurnAvailable || autoTurnEnabled == enabled) return;
    autoTurnEnabled = enabled;
    _send({'type': 'autoTurn', 'enabled': enabled});
    _notify();
  }

  static bool _isSpeaker(String who) => who == 'me' || who == 'them';

  /// The live Partial: stable prefix upright, the rest provisional. Cleared
  /// by the next committed Segment.
  LiveCaption live = LiveCaption.empty;

  /// The translator asked for a pause and is waiting for the next Cut or the
  /// 2.5 s Forced Translation.
  bool held = false;
  int merges = 0;

  /// A Provider error attached to the pending Segment; earlier Segments stay.
  String? inlineError;

  /// The core's error `kind` (auth, quota, unreachable, timeout, …), which
  /// picks the next action shown under [inlineError].
  String? inlineErrorKind;

  /// Set when a Start was refused by the microphone permission; the screens
  /// show a recovery card until the next Start succeeds.
  MicAccess? micRecovery;

  /// An ASR Provider failed: the Session stays alive and this pill reads
  /// "reconnecting" until hypotheses flow again.
  bool asrReconnecting = false;

  /// Listen plays through earphones unless the user opts into the loudspeaker.
  bool loudspeakerOptIn = false;

  String _lastHypothesis = '';
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
      onError: _transportFailed,
    );
    _binarySubscription = _transport.binary.listen(
      _tts.chunk,
      onError: _transportFailed,
    );
    await _connectTransport();
  }

  /// Maps a transport failure to the banner and reports whether the transport
  /// is connected. A Device Host message is already redacted; anything else
  /// can carry the WebSocket URI, token included (web_socket_channel's IO
  /// backend does), so neither the banner nor the log sees it.
  Future<bool> _connectTransport() async {
    try {
      await _transport.connect();
      return true;
    } on DeviceHostException catch (error) {
      // The Device Host redacts Provider keys from its messages, so its
      // specific cause can reach the banner.
      _fail(error.message);
    } catch (_) {
      // Auth and transport failures share one banner; ticket 16 adds the auth
      // message.
      _fail('Connection failed.');
    }
    return false;
  }

  /// A transport stream error: the host behind a live Session failed, so the
  /// mic and playback stop and the banner carries the Device Host's redacted
  /// cause. Any other error can hold the socket URI, token included, so it
  /// keeps the generic banner.
  void _transportFailed(Object error) {
    unawaited(_stopCapture());
    _fail(error is DeviceHostException ? error.message : 'Connection lost.');
  }

  /// The microphone recovery card's "Open Settings".
  Future<void> openMicSettings() => _mic.openSettings();

  /// The onboarding audio check: one utterance through the current output,
  /// outside any Session. False means the platform engine refused.
  Future<bool> testVoice(String text) async {
    try {
      await _systemSpeaker.say(text);
      return true;
    } catch (error) {
      _ttsFailed(error);
      return false;
    }
  }

  void setLoudspeakerOptIn(bool on) {
    loudspeakerOptIn = on;
    _notify();
  }

  /// Loads the Demo sample chips; called by the Start screen when `demo`.
  Future<void> loadSamples() async {
    final loader = _loadSamples;
    try {
      final list = loader != null ? await loader() : await _fetchSamples();
      samples = list;
      _notify();
    } catch (error) {
      this.error = 'Demo samples could not be loaded: $error';
      _notify();
    }
  }

  void selectPair(LanguagePair? pair) {
    selectedPair = pair;
    _notify();
  }

  void selectDirection(DirectionRef? direction) {
    selectedDirection = direction;
    _notify();
  }

  void selectPacks(Set<String> names) {
    selectedPacks = {...names};
    _notify();
  }

  void selectSample(String? id) {
    selectedSampleId = id;
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
      // A Settings Save may have restarted the host and failed, leaving the
      // transport disconnected while this Session still reads `ready`; every
      // frame below would be dropped. Reconnect first so the user sees the
      // host's error instead of a silent, dead listening state.
      if (!await _connectTransport()) return;
      DemoClip? sample;
      if (demo) {
        final sampleId = selectedSampleId ?? 'placeholder';
        if (_sample?.id != sampleId) _sample = null;
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
      final them = isConverse ? pair.reverseOf(direction) : null;
      if (isConverse && (!pair.converse || them == null)) {
        error =
            'This Language Pair translates one way only, so Converse '
            'cannot start. Use Listen instead.';
        _notify();
        return;
      }
      // The permission is asked here, at the first live Start, never at
      // launch; Demo Mode plays a bundled sample and needs no microphone.
      if (!demo) {
        final access = await _mic.request();
        if (access != MicAccess.granted) {
          micRecovery = access;
          _notify();
          return;
        }
      }
      micRecovery = null;
      _meDirection = isConverse ? direction : null;
      _themDirection = them;
      turn = null;
      turnAuto = false;
      autoTurnEnabled = false; // opt-in per Session (S3 no-go)
      autoLanguage = null;
      padCue = 0;
      turnCue = 0;
      turnCueWho = null;
      _gateTail?.cancel();
      gated = false;
      // A new Session restarts seg_id numbering from 1: drop the previous
      // Session's captions, live line and audio, including any unfinished clip.
      _segments.clear();
      live = LiveCaption.empty;
      _lastHypothesis = '';
      held = false;
      merges = 0;
      inlineError = null;
      inlineErrorKind = null;
      asrReconnecting = false;
      _tts.reset();
      _systemTts?.reset();
      _send({
        'type': 'start',
        'mode': isConverse ? 'converse' : 'listen',
        'src': direction.src,
        'tgt': direction.tgt,
        'packs': selectedPacks.toList()..sort(),
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
    await _stopCapture();
    _send({'type': 'stop'});
    turn = null;
    status = SessionStatus.stopped;
    _notify();
  }

  /// Ends local capture and playback; the caller decides the Session status.
  Future<void> _stopCapture() async {
    await _audio.stop();
    await _tts.stop();
    await _systemTts?.stop();
    _gateTail?.cancel();
    gated = false;
  }

  DirectionRef _startDirection(LanguagePair pair, DemoClip? sample) {
    final chosen = selectedDirection;
    if (chosen != null && pair.direction(chosen.src, chosen.tgt) != null) {
      return chosen;
    }
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
      case 'turn':
        _handleTurn(message);
        break;
      case 'asr_partial':
        _handlePartial(message['text'] as String? ?? '');
        break;
      case 'hold':
        _handleHold(message);
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
    // A Settings Save or foreground resume swapped the host under a live
    // Session: the old Session is gone, so stop the hot mic and land on Start.
    if (status == SessionStatus.listening) unawaited(_stopCapture());
    demo = message['demo'] == true;
    tts = message['tts'] as String? ?? '';
    pairs = (message['pairs'] as List<dynamic>? ?? const [])
        .map((pair) => LanguagePair.fromJson(pair as Map<String, dynamic>))
        .toList(growable: false);
    packs = (message['packs'] as List<dynamic>? ?? const [])
        .map((pack) => PackRef.fromJson(pack as Map<String, dynamic>))
        .toList(growable: false);
    presetsInUse = (message['presetsInUse'] as List<dynamic>? ?? const [])
        .map((preset) => PresetRef.fromJson(preset as Map<String, dynamic>))
        .toList(growable: false);
    selectedPair = pairs.isEmpty ? null : pairs.first;
    selectedDirection = null;
    selectedPacks = {};
    _meDirection = null;
    _themDirection = null;
    turn = null;
    turnAuto = false;
    final auto = message['autoTurn'];
    autoTurnAvailable = auto is Map && auto['available'] == true;
    autoTurnEnabled = autoTurnAvailable && auto['enabled'] == true;
    autoLanguage = null;
    padCue = 0;
    _gateTail?.cancel();
    gated = false;
    status = SessionStatus.ready;
    error = null;
    debugPrint(
      'PARLEY hello demo=$demo pairs=${pairs.length} '
      'packs=${packs.length} presets=${presetsInUse.length} tts=$tts',
    );
    _notify();
  }

  void _handlePartial(String text) {
    final stable = _commonPrefix(_lastHypothesis, text);
    _lastHypothesis = text;
    live = LiveCaption(
      stable: stable,
      provisional: text.substring(stable.length),
    );
    inlineError = null;
    inlineErrorKind = null;
    asrReconnecting = false;
    _notify();
  }

  void _handleHold(Map<String, dynamic> message) {
    held = message['held'] == true;
    merges = held ? (message['merges'] as int? ?? 0) : 0;
    _notify();
  }

  void _handleTurn(Map<String, dynamic> message) {
    final who = message['who'] as String?;
    if (!isConverse || who == null || !_isSpeaker(who)) return;
    final previous = turn;
    turn = who;
    turnAuto = message['auto'] == true;
    final language = message['language'];
    if (language is String) autoLanguage = language;
    if (message['override'] == true) {
      debugPrint('PARLEY turn override $who');
    }
    if (message['uncertain'] == true) padCue++;
    debugPrint('PARLEY turn $who auto=$turnAuto language=$autoLanguage');
    if (previous != null && previous != who) {
      turnCue++;
      turnCueWho = who;
    }
    _notify();
  }

  /// Drops the pending live line after the user skips a slow translation.
  void skipLive() {
    live = LiveCaption.empty;
    _lastHypothesis = '';
    _notify();
  }

  void _handleSegment(Map<String, dynamic> message) {
    final segId = message['seg_id'] as int;
    // Committed output is never revised: a repeated seg_id is ignored.
    if (_segments.any((segment) => segment.segId == segId)) return;
    final direction = message['direction'] is Map<String, dynamic>
        ? DirectionRef.fromJson(message['direction'] as Map<String, dynamic>)
        : null;
    final translation = message['tgt'] as String? ?? '';
    final speak = message['speak'] != false;
    if (tts == 'system') {
      // The core's system Provider is text-only: the client speaks the
      // Segment itself and reports the playback it performs.
      if (speak) {
        debugPrint('PARLEY speak $segId lang=${direction?.tgt}');
        _systemSpeaker.speak(segId, translation, language: direction?.tgt);
      }
    } else if (!speak) {
      _tts.silent(segId);
    }
    live = LiveCaption.empty;
    _lastHypothesis = '';
    held = false;
    merges = 0;
    inlineError = null;
    inlineErrorKind = null;
    asrReconnecting = false;
    _segments.add(
      Segment(
        segId: segId,
        source: message['src'] as String? ?? '',
        translation: translation,
        demo: message['demo'] == true,
        latencyMs: message['latency_ms'] as int? ?? 0,
        direction: direction,
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
    if (provider != null) providerErrors[provider] = text;
    if (provider == 'tts') {
      audioError = text;
    } else {
      // Provider failures stay inline with the pending Segment; the transcript
      // above them is never cleared. An ASR failure keeps the Session alive
      // and shows a reconnect state until hypotheses flow again.
      inlineError = text;
      final kind = message['kind'] as String?;
      // The translator reports a bad model/endpoint as `protocol`; the UI's
      // model recovery ("check the model name and Base URL") is the right one.
      inlineErrorKind = provider == 'translator' && kind == 'protocol'
          ? 'model'
          : kind;
      if (provider == 'asr') asrReconnecting = true;
    }
    _notify();
  }

  /// Any TTS failure (audio or system) keeps the transcript and raises the
  /// persistent subtitles-only chip.
  void _ttsFailed(Object error) {
    final message = 'Audio playback failed: $error';
    providerErrors['tts'] = message;
    audioError = message;
    debugPrint('PARLEY tts error $error');
    _notify();
  }

  void _reportPlayback(int segId, String state) {
    debugPrint('PARLEY tts $segId $state');
    if (isConverse) {
      _gateTail?.cancel();
      if (state == 'start') {
        gated = true;
        debugPrint('PARLEY gated true');
      } else {
        // The core keeps Gating for a 500 ms tail after playback ends.
        _gateTail = Timer(const Duration(milliseconds: 500), () {
          gated = false;
          debugPrint('PARLEY gated false');
          if (!_disposed) _notify();
        });
      }
      _notify();
    }
    if (!_disposed) {
      _send({'type': 'playback', 'seg_id': segId, 'state': state});
    }
  }

  void _sendBinary(Uint8List frame) => _transport.sendBinary(frame);

  void _send(Map<String, Object?> message) =>
      _transport.sendText(jsonEncode(message));

  Uri? get _base => _serverBaseProvider?.call() ?? _serverBase;

  Future<List<SampleRef>> _fetchSamples() async {
    final base = _base;
    if (base == null) return const [];
    final response = await _http
        .get(base.resolve('/samples/manifest.json'))
        .timeout(const Duration(seconds: 5));
    if (response.statusCode != 200) {
      throw http.ClientException('HTTP ${response.statusCode}');
    }
    final manifest = jsonDecode(response.body) as Map<String, dynamic>;
    return (manifest['samples'] as List<dynamic>? ?? const [])
        .map((sample) => SampleRef.fromJson(sample as Map<String, dynamic>))
        .toList(growable: false);
  }

  Future<DemoClip?> _fetchDemoClip() async {
    final loader = _loadDemoClip;
    if (loader != null) return loader();
    final base = _base;
    if (base == null) return null;
    final sampleId = selectedSampleId ?? 'placeholder';
    if (!RegExp(r'^[a-z0-9-]+$').hasMatch(sampleId)) {
      error = 'Unknown Demo sample: $sampleId';
      return null;
    }
    try {
      final response = await _http
          .get(base.resolve('/samples/$sampleId.json'))
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
    _gateTail?.cancel();
    _textSubscription?.cancel();
    _binarySubscription?.cancel();
    _audio.stop();
    _tts.dispose();
    _systemTts?.dispose();
    _transport.close();
    _http.close();
    super.dispose();
  }
}

/// The agreed prefix of two ASR hypotheses, compared by code point so a CJK
/// character or emoji is never split.
String _commonPrefix(String a, String b) {
  final left = a.runes.toList(growable: false);
  final right = b.runes.toList(growable: false);
  var index = 0;
  while (index < left.length &&
      index < right.length &&
      left[index] == right[index]) {
    index++;
  }
  return String.fromCharCodes(left.take(index));
}
