import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../model/mode.dart';
import '../model/protocol.dart';
import '../session/listen_session.dart';
import '../theme.dart';
import 'caption_stack.dart';
import 'language_names.dart';

/// The idle screen: Mode, Language Pair and Domain Pack from `hello`, the
/// Provider summary, and Start. One-way pairs are disabled in Converse.
class StartScreen extends StatefulWidget {
  const StartScreen({
    super.key,
    required this.session,
    required this.onStart,
    required this.onSettings,
    this.onUseKeys,
  });

  final ListenSession session;
  final VoidCallback onStart;
  final VoidCallback onSettings;

  /// The Demo banner's "Use my keys" CTA; null on read-only web settings.
  final VoidCallback? onUseKeys;

  @override
  State<StartScreen> createState() => _StartScreenState();
}

class _StartScreenState extends State<StartScreen> {
  SessionMode mode = SessionMode.listen;
  int pairIndex = 0;
  bool swapped = false;
  final Set<String> packs = {};
  String? sampleId;
  bool _samplesRequested = false;

  @override
  void initState() {
    super.initState();
    // Onboarding may have chosen a Mode and Language Pair already.
    mode = widget.session.selectedMode;
    final selected = widget.session.selectedPair;
    final index = selected == null
        ? -1
        : widget.session.pairs.indexOf(selected);
    if (index >= 0) pairIndex = index;
  }

  LanguagePair? get _pair {
    final pairs = widget.session.pairs;
    if (pairs.isEmpty) return null;
    if (pairIndex >= pairs.length) pairIndex = 0;
    return pairs[pairIndex];
  }

  /// Listen reads the translation target ("I read"); the pair's first
  /// Direction is a→b, so swapping picks the reverse path when it exists.
  DirectionRef? _baseDirection(LanguagePair pair) {
    if (pair.directions.isEmpty) return null;
    return pair.direction(pair.a, pair.b) ?? pair.directions.first;
  }

  /// Listen: they speak the source and I read the target. Converse: I speak
  /// the source of my Direction, which is the reverse of the Listen one.
  DirectionRef? _directionFor(LanguagePair pair) {
    final base = _baseDirection(pair);
    if (base == null) return null;
    if (mode == SessionMode.converse) {
      return swapped ? base : pair.reverseOf(base);
    }
    return swapped ? pair.reverseOf(base) : base;
  }

  /// Swap is offered only when the pair has a reverse Direction; a one-way
  /// pair must never enter the swapped state and lose its Direction.
  bool get _canSwap {
    final pair = _pair;
    if (pair == null) return false;
    final base = _baseDirection(pair);
    return base != null && pair.reverseOf(base) != null;
  }

  List<SampleRef> _samplesFor(DirectionRef? direction) {
    if (direction == null) return const [];
    return widget.session.samples
        .where((sample) => sample.matches(direction))
        .toList(growable: false);
  }

  /// The chip that will stream: the explicit choice when it matches the
  /// Direction, else the server's placeholder, else the first match.
  String? _effectiveSampleId(DirectionRef? direction) {
    final samples = _samplesFor(direction);
    if (samples.isEmpty) return null;
    for (final sample in samples) {
      if (sample.id == sampleId) return sample.id;
    }
    for (final sample in samples) {
      if (sample.id == 'placeholder') return sample.id;
    }
    return samples.first.id;
  }

  /// The keyless demo plays the server's bundled sample: their speech in
  /// Listen, my speech in Converse. The sample therefore picks the pair
  /// orientation (`selectSamplePair` in the web client).
  Future<void> _loadSamples() async {
    await widget.session.loadSamples();
    if (!mounted) return;
    _orientToDemo();
  }

  void _orientToDemo() {
    if (!widget.session.demo) return;
    final pair = _pair;
    final base = pair == null ? null : _baseDirection(pair);
    if (pair == null || base == null) return;
    SampleRef? placeholder;
    for (final sample in widget.session.samples) {
      if (sample.id == 'placeholder') {
        placeholder = sample;
        break;
      }
    }
    final sample = placeholder;
    if (sample == null) return;
    final reverse = pair.reverseOf(base);
    final sampleIsBase = sample.matches(base);
    if (!sampleIsBase && (reverse == null || !sample.matches(reverse))) return;
    setState(() {
      // Listen reads the sample as their speech; Converse speaks it as mine.
      swapped = mode == SessionMode.listen ? !sampleIsBase : sampleIsBase;
    });
  }

  void _start(LanguagePair pair, DirectionRef direction) {
    widget.session
      ..selectMode(mode)
      ..selectPair(pair)
      ..selectDirection(direction)
      ..selectPacks({
        for (final pack in widget.session.packs)
          if (pack.matches(direction) && packs.contains(pack.name)) pack.name,
      });
    if (widget.session.demo) {
      widget.session.selectSample(_effectiveSampleId(direction));
    }
    widget.onStart();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.session,
      builder: (context, _) {
        final session = widget.session;
        final colors = ParleyColors.of(context);
        final l10n = AppLocalizations.of(context);
        final pair = _pair;
        final direction = pair == null ? null : _directionFor(pair);
        if (session.demo &&
            session.status == SessionStatus.ready &&
            session.samples.isEmpty &&
            !_samplesRequested) {
          _samplesRequested = true;
          WidgetsBinding.instance.addPostFrameCallback((_) => _loadSamples());
        }
        final oneWayPair = pair != null && !pair.converse;
        final samples = _samplesFor(direction);
        final effectiveSample = _effectiveSampleId(direction);
        final canStart =
            pair != null &&
            direction != null &&
            (mode == SessionMode.listen || pair.converse) &&
            (!session.demo || effectiveSample != null);
        return Scaffold(
          body: SafeArea(
            child: Column(
              children: [
                if (session.demo) DemoBanner(onUseKeys: widget.onUseKeys),
                _Header(onSettings: widget.onSettings),
                Expanded(
                  child: session.status == SessionStatus.connecting
                      ? Center(
                          child: Column(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const CircularProgressIndicator(),
                              const SizedBox(height: 16),
                              Text(l10n.connecting),
                            ],
                          ),
                        )
                      : ListView(
                          padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
                          children: [
                            _ModeSelector(
                              mode: mode,
                              onChanged: (value) {
                                setState(() => mode = value);
                                _orientToDemo();
                              },
                            ),
                            const SizedBox(height: 20),
                            if (session.error != null) ...[
                              _ErrorRow(message: session.error!),
                              const SizedBox(height: 12),
                            ],
                            _PairSelector(
                              session: session,
                              pair: pair,
                              mode: mode,
                              pairIndex: pairIndex,
                              onPair: (index) {
                                setState(() {
                                  pairIndex = index;
                                  swapped = false;
                                });
                                _orientToDemo();
                              },
                              swapped: swapped,
                              canSwap: _canSwap,
                              onSwap: () => setState(() => swapped = !swapped),
                            ),
                            if (mode == SessionMode.converse && oneWayPair) ...[
                              _ReasonRow(
                                key: const Key('listen-only-reason'),
                                text: l10n.listenOnlyPairReason,
                              ),
                              const SizedBox(height: 12),
                            ],
                            if (session.demo) ...[
                              _MicOff(explanation: l10n.demoMicOff),
                              const SizedBox(height: 16),
                              _SampleChips(
                                title: l10n.demoSamples,
                                samples: samples,
                                selected: effectiveSample,
                                onSelected: (id) =>
                                    setState(() => sampleId = id),
                                missing: l10n.sampleMissing,
                              ),
                              const SizedBox(height: 16),
                            ],
                            _PackChips(
                              title: l10n.domainPacks,
                              packs: [
                                for (final pack in session.packs)
                                  if (direction != null &&
                                      pack.matches(direction))
                                    pack,
                              ],
                              selected: packs,
                              onToggle: (name) => setState(() {
                                packs.contains(name)
                                    ? packs.remove(name)
                                    : packs.add(name);
                              }),
                              empty: l10n.noDomainPacks,
                            ),
                            const SizedBox(height: 20),
                            _ProviderSummary(session: session),
                            const SizedBox(height: 24),
                            Container(
                              height: 120,
                              alignment: Alignment.center,
                              decoration: BoxDecoration(
                                color: colors.surface,
                                borderRadius: BorderRadius.circular(12),
                              ),
                              child: Text(
                                l10n.captionsEmpty,
                                style: TextStyle(color: colors.muted),
                              ),
                            ),
                          ],
                        ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 0, 16, 4),
                  child: Text(
                    l10n.micPrimer,
                    key: const Key('mic-primer'),
                    style: TextStyle(color: colors.muted, fontSize: 12),
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
                  child: SizedBox(
                    width: double.infinity,
                    height: 56,
                    child: FilledButton.icon(
                      key: const Key('start-button'),
                      onPressed: canStart
                          ? () => _start(pair, direction)
                          : null,
                      icon: const Icon(Icons.play_arrow),
                      label: Text(
                        mode == SessionMode.listen
                            ? l10n.startListen
                            : l10n.startConverse,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.onSettings});

  final VoidCallback onSettings;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    final l10n = AppLocalizations.of(context);
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 8, 4),
      child: Row(
        children: [
          Text(
            'Parley',
            style: TextStyle(
              color: colors.ink,
              fontSize: 22,
              fontWeight: FontWeight.w600,
            ),
          ),
          const Spacer(),
          IconButton(
            key: const Key('open-settings'),
            tooltip: l10n.settings,
            onPressed: onSettings,
            icon: Icon(Icons.settings_outlined, color: colors.ink),
          ),
        ],
      ),
    );
  }
}

class _ModeSelector extends StatelessWidget {
  const _ModeSelector({required this.mode, required this.onChanged});

  final SessionMode mode;
  final ValueChanged<SessionMode> onChanged;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Wrap(
      spacing: 8,
      children: [
        ChoiceChip(
          key: const Key('mode-listen'),
          avatar: const Icon(Icons.headphones_outlined, size: 18),
          label: Text(l10n.modeListen),
          selected: mode == SessionMode.listen,
          onSelected: (_) => onChanged(SessionMode.listen),
        ),
        ChoiceChip(
          key: const Key('mode-converse'),
          avatar: const Icon(Icons.record_voice_over_outlined, size: 18),
          label: Text(l10n.modeConverse),
          selected: mode == SessionMode.converse,
          onSelected: (_) => onChanged(SessionMode.converse),
        ),
      ],
    );
  }
}

class _PairSelector extends StatelessWidget {
  const _PairSelector({
    required this.session,
    required this.pair,
    required this.mode,
    required this.pairIndex,
    required this.onPair,
    required this.swapped,
    required this.canSwap,
    required this.onSwap,
  });

  final ListenSession session;
  final LanguagePair? pair;
  final SessionMode mode;
  final int pairIndex;
  final ValueChanged<int> onPair;
  final bool swapped;
  final bool canSwap;
  final VoidCallback onSwap;

  /// A disabled Converse chip still says why: the one-way reason is its
  /// tooltip, not only the row that appears when it is selected.
  Widget _pairChip(BuildContext context, int index) {
    final l10n = AppLocalizations.of(context);
    final disabled =
        mode == SessionMode.converse && !session.pairs[index].converse;
    final chip = ChoiceChip(
      key: Key('pair-$index'),
      label: Text(
        '${languageName(context, session.pairs[index].a)} '
        '↔ ${languageName(context, session.pairs[index].b)}',
      ),
      selected: index == pairIndex,
      onSelected: disabled ? null : (_) => onPair(index),
    );
    return disabled
        ? Tooltip(message: l10n.listenOnlyPairReason, child: chip)
        : chip;
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    if (pair == null) return const SizedBox.shrink();
    final source = languageName(context, swapped ? pair!.b : pair!.a);
    final target = languageName(context, swapped ? pair!.a : pair!.b);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Wrap(
          spacing: 8,
          runSpacing: 4,
          children: [
            for (var index = 0; index < session.pairs.length; index++)
              _pairChip(context, index),
            IconButton(
              key: const Key('swap-languages'),
              tooltip: canSwap ? l10n.swapLanguages : l10n.oneWayPair,
              onPressed: canSwap ? onSwap : null,
              icon: const Icon(Icons.swap_horiz),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          mode == SessionMode.listen
              ? l10n.startPairListen(source, target)
              : l10n.startPairConverse(target, source),
          key: const Key('pair-direction'),
          style: Theme.of(context).textTheme.titleMedium,
        ),
      ],
    );
  }
}

class _PackChips extends StatelessWidget {
  const _PackChips({
    required this.title,
    required this.packs,
    required this.selected,
    required this.onToggle,
    required this.empty,
  });

  final String title;
  final List<PackRef> packs;
  final Set<String> selected;
  final ValueChanged<String> onToggle;
  final String empty;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title, style: Theme.of(context).textTheme.labelLarge),
        const SizedBox(height: 8),
        if (packs.isEmpty)
          Text(empty, style: TextStyle(color: ParleyColors.of(context).muted))
        else
          Wrap(
            spacing: 8,
            children: [
              for (final pack in packs)
                FilterChip(
                  key: Key('pack-${pack.name}'),
                  label: Text(pack.name),
                  selected: selected.contains(pack.name),
                  onSelected: (_) => onToggle(pack.name),
                ),
            ],
          ),
      ],
    );
  }
}

class _SampleChips extends StatelessWidget {
  const _SampleChips({
    required this.title,
    required this.samples,
    required this.selected,
    required this.onSelected,
    required this.missing,
  });

  final String title;
  final List<SampleRef> samples;
  final String? selected;
  final ValueChanged<String> onSelected;
  final String missing;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title, style: Theme.of(context).textTheme.labelLarge),
        const SizedBox(height: 8),
        if (samples.isEmpty)
          Text(missing, style: TextStyle(color: ParleyColors.of(context).muted))
        else
          Wrap(
            spacing: 8,
            children: [
              for (final sample in samples)
                ChoiceChip(
                  key: Key('sample-${sample.id}'),
                  label: Text(sample.id),
                  selected: sample.id == selected,
                  onSelected: (_) => onSelected(sample.id),
                ),
            ],
          ),
      ],
    );
  }
}

class _MicOff extends StatelessWidget {
  const _MicOff({required this.explanation});

  final String explanation;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Row(
      key: const Key('mic-off-demo'),
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(Icons.mic_off_outlined, size: 18, color: colors.muted),
        const SizedBox(width: 8),
        Expanded(
          child: Text(explanation, style: TextStyle(color: colors.muted)),
        ),
      ],
    );
  }
}

class _ProviderSummary extends StatelessWidget {
  const _ProviderSummary({required this.session});

  final ListenSession session;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final colors = ParleyColors.of(context);
    final presets = session.presetsInUse.map((preset) => preset.id).toList();
    final training = session.presetsInUse
        .where((preset) => preset.trainsOnData)
        .map((preset) => preset.id)
        .toList();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(Icons.dns_outlined, size: 18, color: colors.muted),
            const SizedBox(width: 8),
            Text(
              l10n.providersTitle,
              style: Theme.of(context).textTheme.labelLarge,
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                session.demo
                    ? l10n.providerDemo
                    : [
                        if (presets.isNotEmpty) presets.join(', '),
                        session.tts == 'audio'
                            ? l10n.settingsServerDefault
                            : l10n.settingsVoiceSystem,
                      ].join(' · '),
                style: TextStyle(color: colors.muted),
              ),
            ),
          ],
        ),
        if (training.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(Icons.info_outline, size: 18, color: colors.ink),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    l10n.presetTrainsOnData(training.join(', ')),
                    style: TextStyle(color: colors.ink),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

class _ReasonRow extends StatelessWidget {
  const _ReasonRow({super.key, required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(Icons.info_outline, size: 18, color: colors.muted),
        const SizedBox(width: 8),
        Expanded(
          child: Text(text, style: TextStyle(color: colors.muted)),
        ),
      ],
    );
  }
}

class _ErrorRow extends StatelessWidget {
  const _ErrorRow({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(Icons.error_outline, size: 18, color: colors.error),
        const SizedBox(width: 8),
        Expanded(
          child: Text(message, style: TextStyle(color: colors.error)),
        ),
      ],
    );
  }
}
