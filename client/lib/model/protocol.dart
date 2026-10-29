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

  /// The Direction that starts at [src] and ends at [tgt], or null.
  DirectionRef? direction(String src, String tgt) {
    for (final direction in directions) {
      if (direction.src == src && direction.tgt == tgt) return direction;
    }
    return null;
  }

  /// The Direction opposite to [direction], or null for a one-way pair.
  DirectionRef? reverseOf(DirectionRef direction) =>
      this.direction(direction.tgt, direction.src);
}

/// One Domain Pack the server offers for a Direction (`hello.packs`).
class PackRef {
  const PackRef({required this.name, required this.src, required this.tgt});

  factory PackRef.fromJson(Map<String, dynamic> json) => PackRef(
    name: json['name'] as String,
    src: json['src'] as String,
    tgt: json['tgt'] as String,
  );

  final String name;
  final String src;
  final String tgt;

  bool matches(DirectionRef direction) =>
      src == direction.src && tgt == direction.tgt;
}

/// One bundled Demo sample from `/samples/manifest.json`.
class SampleRef {
  const SampleRef({required this.id, required this.src, required this.tgt});

  factory SampleRef.fromJson(Map<String, dynamic> json) => SampleRef(
    id: json['id'] as String,
    src: json['src'] as String,
    tgt: json['tgt'] as String,
  );

  final String id;
  final String src;
  final String tgt;

  bool matches(DirectionRef direction) =>
      src == direction.src && tgt == direction.tgt;
}

/// One Preset the running Session uses (`hello.presetsInUse`).
class PresetRef {
  const PresetRef({required this.id, required this.trainsOnData});

  factory PresetRef.fromJson(Map<String, dynamic> json) => PresetRef(
    id: json['id'] as String,
    trainsOnData: json['trainsOnData'] == true,
  );

  final String id;
  final bool trainsOnData;
}

/// One committed Segment: the source line and its translation, never revised.
class Segment {
  const Segment({
    required this.segId,
    required this.source,
    required this.translation,
    required this.demo,
    this.latencyMs = 0,
    this.direction,
  });

  final int segId;
  final String source;
  final String translation;
  final bool demo;

  /// Speech end → Commit, shown only under Advanced.
  final int latencyMs;

  /// The Direction the Segment was translated along. Its languages set each
  /// line's `TextStyle.locale` so Han glyphs keep their regional forms.
  final DirectionRef? direction;
}

/// The display-only ASR hypothesis under the committed Segments. The prefix
/// agreed with the previous hypothesis is shown upright, the rest greyed; the
/// whole line is hidden from screen readers and never translated.
class LiveCaption {
  const LiveCaption({this.stable = '', this.provisional = ''});

  static const empty = LiveCaption();

  final String stable;
  final String provisional;

  String get text => '$stable$provisional';
  bool get isEmpty => text.isEmpty;
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
