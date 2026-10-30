import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../model/mode.dart';
import '../model/protocol.dart';
import '../session/listen_session.dart';
import '../theme.dart';
import 'language_names.dart';

/// The three-step first run: pair, try it, check audio. The keyless Demo path
/// starts the Session straight from the last step; the keys and server paths
/// land on Start, where their setup lives.
class OnboardingScreen extends StatefulWidget {
  const OnboardingScreen({
    super.key,
    required this.session,
    required this.onFinish,
    required this.onSettings,
    this.onUseKeys,
  });

  final ListenSession session;

  /// startNow is true only for the Demo, which needs no further setup.
  final void Function({required bool startNow}) onFinish;
  final VoidCallback onSettings;
  final VoidCallback? onUseKeys;

  @override
  State<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends State<OnboardingScreen> {
  int _step = 0;
  int _pairIndex = 0;
  SessionMode _mode = SessionMode.listen;
  String _path = 'server';
  bool _pathTouched = false;
  bool _loudspeaker = false;
  bool _testFailed = false;

  @override
  void initState() {
    super.initState();
    _mode = widget.session.selectedMode;
    _pairIndex = _indexOfSelectedPair();
    _loudspeaker = widget.session.loudspeakerOptIn;
  }

  int _indexOfSelectedPair() {
    final selected = widget.session.selectedPair;
    final pairs = widget.session.pairs;
    if (selected == null) return 0;
    final index = pairs.indexOf(selected);
    return index < 0 ? 0 : index;
  }

  LanguagePair? get _pair {
    final pairs = widget.session.pairs;
    if (pairs.isEmpty) return null;
    return pairs[_pairIndex.clamp(0, pairs.length - 1)];
  }

  String get _effectivePath =>
      !_pathTouched && widget.session.demo ? 'demo' : _path;

  void _selectPair(int index) {
    setState(() {
      _pairIndex = index;
      if (_mode == SessionMode.converse &&
          !widget.session.pairs[index].converse) {
        _mode = SessionMode.listen;
      }
    });
  }

  void _finish() {
    final pair = _pair;
    if (pair == null) return;
    widget.session.selectMode(_mode);
    widget.session.selectPair(pair);
    widget.session.setLoudspeakerOptIn(_loudspeaker);
    widget.onFinish(startNow: _effectivePath == 'demo' && widget.session.demo);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.session,
      builder: (context, _) {
        final l10n = AppLocalizations.of(context);
        final colors = ParleyColors.of(context);
        return Scaffold(
          backgroundColor: colors.bg,
          body: SafeArea(
            child: Column(
              children: [
                Padding(
                  padding: EdgeInsets.fromLTRB(
                    parleyMargin(context),
                    20,
                    parleyMargin(context),
                    8,
                  ),
                  child: Row(
                    children: [
                      for (var i = 0; i < 3; i++)
                        Container(
                          key: Key('onboarding-dot-$i'),
                          width: i == _step ? 24 : 8,
                          height: 8,
                          margin: const EdgeInsets.only(right: 6),
                          decoration: BoxDecoration(
                            color: i == _step ? colors.ink : colors.hairline,
                            borderRadius: BorderRadius.circular(999),
                          ),
                        ),
                    ],
                  ),
                ),
                Expanded(
                  child: SingleChildScrollView(
                    padding: EdgeInsets.fromLTRB(
                      parleyMargin(context),
                      8,
                      parleyMargin(context),
                      16,
                    ),
                    child: switch (_step) {
                      0 => _pairStep(l10n, colors),
                      1 => _tryStep(l10n, colors),
                      _ => _audioStep(l10n, colors),
                    },
                  ),
                ),
                Padding(
                  padding: EdgeInsets.fromLTRB(
                    parleyMargin(context),
                    8,
                    parleyMargin(context),
                    16,
                  ),
                  child: Row(
                    children: [
                      if (_step > 0)
                        TextButton(
                          key: const Key('onboarding-back'),
                          onPressed: () => setState(() => _step--),
                          child: Text(l10n.onboardingBack),
                        ),
                      const Spacer(),
                      FilledButton(
                        key: _step == 2
                            ? const Key('onboarding-finish')
                            : const Key('onboarding-next'),
                        onPressed: _step == 2
                            ? (_pair == null ? null : _finish)
                            : (_pair == null
                                  ? null
                                  : () => setState(() => _step++)),
                        child: Text(
                          _step == 2
                              ? (_effectivePath == 'demo' && widget.session.demo
                                    ? l10n.onboardingStartDemo
                                    : l10n.onboardingFinish)
                              : l10n.onboardingNext,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget _pairStep(AppLocalizations l10n, ParleyColors colors) {
    final pairs = widget.session.pairs;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _title(l10n.onboardingPairTitle, colors),
        const SizedBox(height: 8),
        Text(
          l10n.onboardingPairBody,
          style: TextStyle(color: colors.muted, fontSize: 14),
        ),
        const SizedBox(height: 16),
        Wrap(
          spacing: 8,
          children: [
            ChoiceChip(
              key: const Key('onboarding-mode-listen'),
              label: Text(l10n.modeListen),
              selected: _mode == SessionMode.listen,
              onSelected: (_) => setState(() => _mode = SessionMode.listen),
            ),
            ChoiceChip(
              key: const Key('onboarding-mode-converse'),
              label: Text(l10n.modeConverse),
              selected: _mode == SessionMode.converse,
              onSelected: (_pair?.converse ?? false)
                  ? (_) => setState(() => _mode = SessionMode.converse)
                  : null,
            ),
          ],
        ),
        const SizedBox(height: 16),
        if (pairs.isEmpty)
          Text(
            l10n.connecting,
            key: const Key('onboarding-waiting'),
            style: TextStyle(color: colors.muted),
          )
        else
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              for (var i = 0; i < pairs.length; i++)
                ChoiceChip(
                  key: Key('onboarding-pair-$i'),
                  label: Text(
                    '${languageName(context, pairs[i].a)} '
                    '↔ ${languageName(context, pairs[i].b)}',
                  ),
                  selected: i == _pairIndex,
                  onSelected: (_) => _selectPair(i),
                ),
            ],
          ),
      ],
    );
  }

  Widget _tryStep(AppLocalizations l10n, ParleyColors colors) {
    final path = _effectivePath;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _title(l10n.onboardingTryTitle, colors),
        const SizedBox(height: 8),
        Text(
          l10n.onboardingTryBody,
          style: TextStyle(color: colors.muted, fontSize: 14),
        ),
        const SizedBox(height: 16),
        _TryCard(
          key: const Key('onboarding-try-demo'),
          title: l10n.onboardingDemo,
          body: widget.session.demo
              ? l10n.onboardingDemoBody
              : l10n.onboardingNoDemo,
          selected: path == 'demo',
          onTap: widget.session.demo
              ? () => setState(() {
                  _path = 'demo';
                  _pathTouched = true;
                })
              : null,
          colors: colors,
        ),
        _TryCard(
          key: const Key('onboarding-try-keys'),
          title: l10n.onboardingKeys,
          body: l10n.onboardingKeysBody,
          selected: path == 'keys',
          onTap: () => setState(() {
            _path = 'keys';
            _pathTouched = true;
          }),
          action: widget.onUseKeys == null
              ? null
              : TextButton(
                  key: const Key('onboarding-open-settings'),
                  onPressed: widget.onUseKeys,
                  child: Text(l10n.openSettings),
                ),
          colors: colors,
        ),
        _TryCard(
          key: const Key('onboarding-try-server'),
          title: l10n.onboardingServer,
          body: l10n.onboardingServerBody,
          selected: path == 'server',
          onTap: () => setState(() {
            _path = 'server';
            _pathTouched = true;
          }),
          colors: colors,
        ),
      ],
    );
  }

  Widget _audioStep(AppLocalizations l10n, ParleyColors colors) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _title(l10n.onboardingAudioTitle, colors),
        const SizedBox(height: 8),
        Text(
          l10n.onboardingAudioBody,
          key: const Key('mic-primer'),
          style: TextStyle(color: colors.muted, fontSize: 14),
        ),
        const SizedBox(height: 20),
        Wrap(
          spacing: 8,
          children: [
            ChoiceChip(
              key: const Key('onboarding-output-earphones'),
              label: Text(l10n.onboardingEarphones),
              selected: !_loudspeaker,
              onSelected: (_) => setState(() => _loudspeaker = false),
            ),
            ChoiceChip(
              key: const Key('onboarding-output-loudspeaker'),
              label: Text(l10n.onboardingLoudspeaker),
              selected: _loudspeaker,
              onSelected: (_) => setState(() => _loudspeaker = true),
            ),
          ],
        ),
        const SizedBox(height: 12),
        Text(
          _loudspeaker ? l10n.listenLoudspeakerOn : l10n.listenEarphonesHint,
          style: TextStyle(color: colors.muted, fontSize: 12),
        ),
        const SizedBox(height: 8),
        TextButton.icon(
          key: const Key('onboarding-test-sound'),
          onPressed: _testSound,
          icon: const Icon(Icons.volume_up_outlined),
          label: Text(l10n.onboardingTestSound),
        ),
        if (_testFailed)
          Text(
            l10n.audioTestFailed,
            key: const Key('onboarding-test-failed'),
            style: TextStyle(color: colors.error, fontSize: 12),
          ),
      ],
    );
  }

  Future<void> _testSound() async {
    final l10n = AppLocalizations.of(context);
    final ok = await widget.session.testVoice(l10n.audioTestPhrase);
    if (mounted) setState(() => _testFailed = !ok);
  }

  Widget _title(String text, ParleyColors colors) => Text(
    text,
    style: TextStyle(
      color: colors.ink,
      fontSize: 28,
      fontWeight: FontWeight.w600,
    ),
  );
}

class _TryCard extends StatelessWidget {
  const _TryCard({
    super.key,
    required this.title,
    required this.body,
    required this.selected,
    required this.onTap,
    required this.colors,
    this.action,
  });

  final String title;
  final String body;
  final bool selected;
  final VoidCallback? onTap;
  final ParleyColors colors;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final enabled = onTap != null;
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: colors.surface,
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: selected ? colors.ink : colors.hairline,
              width: selected ? 2 : 1,
            ),
          ),
          child: Row(
            children: [
              Icon(
                selected
                    ? Icons.radio_button_checked
                    : Icons.radio_button_unchecked,
                size: 20,
                color: enabled ? colors.ink : colors.muted,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: TextStyle(
                        color: enabled ? colors.ink : colors.muted,
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    Text(
                      body,
                      style: TextStyle(color: colors.muted, fontSize: 13),
                    ),
                  ],
                ),
              ),
              ?action,
            ],
          ),
        ),
      ),
    );
  }
}
