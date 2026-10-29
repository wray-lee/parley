import 'dart:math' as math;
import 'dart:typed_data';

/// A bilingual Direction as the server names it in `hello`.
class DirectionRef {
  const DirectionRef({required this.src, required this.tgt});

  factory DirectionRef.fromJson(Map<String, dynamic> json) =>
      DirectionRef(src: json['src'] as String, tgt: json['tgt'] as String);

  final String src;
  final String tgt;
}

/// A Language Pair from `hello.pairs`.
class LanguagePair {
  const LanguagePair({
    required this.a,
    required this.b,
    required this.directions,
    required this.converse,
  });

  factory LanguagePair.fromJson(Map<String, dynamic> json) => LanguagePair(
    a: json['a'] as String,
    b: json['b'] as String,
    directions: (json['directions'] as List<dynamic>)
        .map((d) => DirectionRef.fromJson(d as Map<String, dynamic>))
        .toList(growable: false),
    converse: json['converse'] == true,
  );

  final String a;
  final String b;
  final List<DirectionRef> directions;
  final bool converse;

  String get label => '$a ↔ $b';
}

/// One committed Segment: the source line and its translation, never revised.
class Segment {
  const Segment({
    required this.segId,
    required this.source,
    required this.translation,
    required this.demo,
  });

  final int segId;
  final String source;
  final String translation;
  final bool demo;
}

/// The bundled Demo clip the Server Host advertises at `/samples/placeholder.json`.
/// Its waveform is synthesised from the energy spans; the mock ASR only needs
/// the frame clock, the VAD needs the silence gaps.
class DemoClip {
  const DemoClip({
    required this.id,
    required this.durationMs,
    required this.spans,
    required this.src,
    required this.tgt,
  });

  factory DemoClip.fromJson(Map<String, dynamic> json) => DemoClip(
    id: json['id'] as String,
    durationMs: json['duration_ms'] as int,
    spans: (json['audio'] as List<dynamic>? ?? const [])
        .map((s) => DemoClipSpan.fromJson(s as Map<String, dynamic>))
        .toList(growable: false),
    src: json['src'] as String,
    tgt: json['tgt'] as String,
  );

  static const int sampleRate = 16000;

  final String id;
  final int durationMs;
  final List<DemoClipSpan> spans;
  final String src;
  final String tgt;

  /// The 16 kHz PCM16 frame starting at [offsetMs], as the lite web client
  /// synthesises it (`web/demo-sample.js`).
  Int16List frame(int offsetMs, int samples) {
    final pcm = Int16List(samples);
    for (var i = 0; i < samples; i++) {
      final timeMs = offsetMs + (i * 1000) / sampleRate;
      final amplitude = _amplitudeAt(timeMs);
      if (amplitude != 0) {
        pcm[i] =
            (math.sin(2 * math.pi * 220 * timeMs / 1000) * amplitude * 32767)
                .round();
      }
    }
    return pcm;
  }

  double _amplitudeAt(double timeMs) {
    for (final span in spans) {
      if (timeMs >= span.startMs && timeMs < span.endMs) return span.amplitude;
    }
    return 0;
  }
}

class DemoClipSpan {
  const DemoClipSpan({
    required this.startMs,
    required this.endMs,
    required this.amplitude,
  });

  factory DemoClipSpan.fromJson(Map<String, dynamic> json) => DemoClipSpan(
    startMs: json['start_ms'] as int,
    endMs: json['end_ms'] as int,
    amplitude: (json['amplitude'] as num).toDouble(),
  );

  final int startMs;
  final int endMs;
  final double amplitude;
}
