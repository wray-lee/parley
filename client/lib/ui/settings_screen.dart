import 'package:material_ui/material_ui.dart';

import '../l10n/app_localizations.dart';
import '../platform/local_network_permission.dart';
import '../session/listen_session.dart';
import '../settings/settings_controller.dart';
import '../settings/validation.dart';
import '../theme.dart';

/// Provider settings. Native builds edit keys (stored by [SettingsController]
/// in secure storage); the web build is a read-only summary from `hello` with
/// no key and no server-URL field.
class SettingsScreen extends StatefulWidget {
  const SettingsScreen({
    super.key,
    required this.session,
    this.controller,
    this.readOnly = false,
    this.localNetwork,
  });

  final ListenSession session;
  final SettingsController? controller;
  final bool readOnly;

  /// Android 17 asks for local-network access only when a LAN URL is saved.
  final LocalNetworkPermission? localNetwork;

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  late Settings draft = widget.controller?.values ?? const Settings();
  bool editingLlm = false;
  bool editingAsr = false;
  bool editingAzure = false;
  String? saveMessage;
  bool _localNetworkDenied = false;

  SettingsController? get controller => widget.controller;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final readOnly = widget.readOnly || controller == null;
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.settings),
        leading: IconButton(
          key: const Key('close-settings'),
          tooltip: l10n.settingsClose,
          icon: const Icon(Icons.close),
          onPressed: () => Navigator.of(context).maybePop(),
        ),
        actions: [
          if (!readOnly)
            TextButton(
              key: const Key('save-settings'),
              onPressed: _save,
              child: Text(l10n.settingsSave),
            ),
        ],
      ),
      body: readOnly
          ? _ReadOnlySummary(session: widget.session)
          : ListView(
              padding: const EdgeInsets.all(16),
              children: [
                if (controller!.hostError != null) ...[
                  Text(
                    controller!.hostError!,
                    key: const Key('settings-host-error'),
                    style: TextStyle(color: ParleyColors.of(context).error),
                  ),
                  const SizedBox(height: 8),
                ],
                _Card(
                  icon: Icons.record_voice_over_outlined,
                  title: l10n.settingsSpeech,
                  children: [
                    DropdownButtonFormField<String>(
                      key: const Key('asr-provider'),
                      initialValue: draft.asrProvider,
                      decoration: const InputDecoration(labelText: ''),
                      items: [
                        DropdownMenuItem(
                          value: 'deepgram',
                          child: Text(l10n.providerDeepgram),
                        ),
                        DropdownMenuItem(
                          value: 'paraformer',
                          child: Text(l10n.providerParaformer),
                        ),
                        DropdownMenuItem(
                          value: 'mock',
                          child: Text(l10n.providerMock),
                        ),
                      ],
                      onChanged: (value) {
                        if (value == null) return;
                        setState(() {
                          draft = draft.copyWith(asrProvider: value);
                          editingAsr = false;
                        });
                      },
                    ),
                    if (draft.asrProvider == 'mock')
                      _Hint(l10n.settingsMockNoKey)
                    else
                      _SecretField(
                        field: 'asr',
                        label: l10n.settingsApiKey,
                        value: draft.asrProvider == 'deepgram'
                            ? draft.deepgramKey
                            : draft.paraformerKey,
                        editing: editingAsr,
                        onEdit: () => setState(() => editingAsr = true),
                        onClear: () => setState(() {
                          draft = draft.asrProvider == 'deepgram'
                              ? draft.copyWith(deepgramKey: '')
                              : draft.copyWith(paraformerKey: '');
                          editingAsr = false;
                        }),
                        onChanged: (value) => setState(() {
                          draft = draft.asrProvider == 'deepgram'
                              ? draft.copyWith(deepgramKey: value)
                              : draft.copyWith(paraformerKey: value);
                        }),
                      ),
                    _ProviderError(
                      message: widget.session.providerErrors['asr'],
                    ),
                  ],
                ),
                const SizedBox(height: 16),
                _Card(
                  icon: Icons.translate,
                  title: l10n.settingsTranslation,
                  children: [
                    DropdownButtonFormField<String>(
                      key: const Key('translator-preset'),
                      initialValue: draft.translatorPreset,
                      decoration: InputDecoration(
                        labelText: l10n.settingsPreset,
                      ),
                      // One item per catalog Preset: `custom` is a real entry
                      // in config/presets.json, only its label is localized.
                      items: [
                        for (final id in controller!.presets.presets.keys)
                          DropdownMenuItem(
                            value: id,
                            child: Text(
                              id == 'custom' ? l10n.settingsPresetCustom : id,
                            ),
                          ),
                      ],
                      onChanged: (value) {
                        if (value == null) return;
                        setState(() {
                          draft = draft.copyWith(translatorPreset: value);
                        });
                      },
                    ),
                    _TextInput(
                      key: const Key('translator-base-url'),
                      label: l10n.settingsBaseUrl,
                      value: draft.translatorBaseUrl,
                      error: _errorText(l10n, controller!.errors['baseUrl']),
                      onChanged: (value) => setState(() {
                        draft = draft.copyWith(translatorBaseUrl: value);
                      }),
                    ),
                    _TextInput(
                      key: const Key('translator-model'),
                      label: l10n.settingsModel,
                      value: draft.translatorModel,
                      error: _errorText(l10n, controller!.errors['model']),
                      onChanged: (value) => setState(() {
                        draft = draft.copyWith(translatorModel: value);
                      }),
                    ),
                    _SecretField(
                      field: 'llm',
                      label: l10n.settingsApiKey,
                      value: draft.llmKey,
                      editing: editingLlm,
                      onEdit: () => setState(() => editingLlm = true),
                      onClear: () => setState(() {
                        draft = draft.copyWith(llmKey: '');
                        editingLlm = false;
                      }),
                      onChanged: (value) =>
                          setState(() => draft = draft.copyWith(llmKey: value)),
                    ),
                    ExpansionTile(
                      title: Text(l10n.settingsAdvanced),
                      tilePadding: EdgeInsets.zero,
                      children: [
                        _TextInput(
                          key: const Key('translator-extra-body'),
                          label: l10n.settingsExtraBody,
                          value: draft.translatorExtraBody,
                          error: _errorText(
                            l10n,
                            controller!.errors['extraBody'],
                          ),
                          onChanged: (value) => setState(() {
                            draft = draft.copyWith(translatorExtraBody: value);
                          }),
                        ),
                      ],
                    ),
                    _ProviderError(
                      message: widget.session.providerErrors['translator'],
                    ),
                  ],
                ),
                const SizedBox(height: 16),
                _Card(
                  icon: Icons.volume_up_outlined,
                  title: l10n.settingsVoice,
                  children: [
                    DropdownButtonFormField<String>(
                      key: const Key('tts-provider'),
                      initialValue: draft.ttsProvider,
                      decoration: const InputDecoration(labelText: ''),
                      items: [
                        DropdownMenuItem(
                          value: 'system',
                          child: Text(l10n.settingsVoiceSystem),
                        ),
                        DropdownMenuItem(
                          value: 'azure',
                          child: Text(l10n.providerAzure),
                        ),
                        DropdownMenuItem(
                          value: 'kokoro',
                          child: Text(l10n.providerKokoro),
                        ),
                        DropdownMenuItem(
                          value: 'mock',
                          child: Text(l10n.providerMock),
                        ),
                      ],
                      onChanged: (value) {
                        if (value == null) return;
                        setState(() {
                          draft = draft.copyWith(ttsProvider: value);
                          editingAzure = false;
                        });
                      },
                    ),
                    if (draft.ttsProvider == 'azure') ...[
                      _SecretField(
                        field: 'azure',
                        label: l10n.settingsApiKey,
                        value: draft.azureKey,
                        editing: editingAzure,
                        onEdit: () => setState(() => editingAzure = true),
                        onClear: () => setState(() {
                          draft = draft.copyWith(azureKey: '');
                          editingAzure = false;
                        }),
                        onChanged: (value) => setState(() {
                          draft = draft.copyWith(azureKey: value);
                        }),
                      ),
                      _TextInput(
                        key: const Key('azure-region'),
                        label: l10n.settingsRegion,
                        value: draft.azureRegion,
                        onChanged: (value) => setState(() {
                          draft = draft.copyWith(azureRegion: value);
                        }),
                      ),
                    ],
                    if (draft.ttsProvider == 'kokoro')
                      _TextInput(
                        key: const Key('kokoro-url'),
                        label: l10n.settingsKokoroUrl,
                        value: draft.kokoroUrl,
                        onChanged: (value) => setState(() {
                          draft = draft.copyWith(kokoroUrl: value);
                        }),
                      ),
                    if (draft.ttsProvider == 'mock')
                      _Hint(l10n.settingsMockNoKey),
                    _ProviderError(
                      message: widget.session.providerErrors['tts'],
                    ),
                  ],
                ),
                const SizedBox(height: 16),
                _Hint(l10n.settingsSaveRestartsHost),
                if (saveMessage != null) ...[
                  const SizedBox(height: 8),
                  Text(
                    saveMessage!,
                    key: const Key('settings-save-message'),
                    style: TextStyle(
                      color:
                          controller!.saved &&
                              controller!.hostError == null &&
                              !_localNetworkDenied
                          ? ParleyColors.of(context).ink
                          : ParleyColors.of(context).error,
                    ),
                  ),
                ],
              ],
            ),
    );
  }

  Future<void> _save() async {
    final controller = this.controller;
    if (controller == null) return;
    controller.update(draft);
    final l10n = AppLocalizations.of(context);
    final saved = await controller.save();
    var localNetworkDenied = false;
    if (saved && needsLocalNetworkPermission(controller.values.toMap())) {
      final access =
          await (widget.localNetwork ?? createLocalNetworkPermission())
              .request();
      localNetworkDenied = access == LocalNetworkAccess.denied;
    }
    if (!mounted) return;
    setState(() {
      _localNetworkDenied = localNetworkDenied;
      saveMessage = localNetworkDenied
          ? l10n.localNetworkDenied
          : controller.hostError ??
                (saved ? l10n.settingsSaved : l10n.settingsSaveFailed);
    });
  }

  String? _errorText(AppLocalizations l10n, String? code) => switch (code) {
    'needsBaseUrl' => l10n.errorNeedsBaseUrl,
    'invalidUrl' => l10n.errorInvalidUrl,
    'notHttp' => l10n.errorNotHttp,
    'plainHttp' => l10n.errorPlainHttp,
    'credentials' => l10n.errorCredentials,
    'query' => l10n.errorQuery,
    'needsModel' => l10n.errorNeedsModel,
    'invalidJson' => l10n.errorInvalidJson,
    'notObject' => l10n.errorNotObject,
    _ => null,
  };
}

class _ReadOnlySummary extends StatelessWidget {
  const _ReadOnlySummary({required this.session});

  final ListenSession session;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final colors = ParleyColors.of(context);
    final presets = session.presetsInUse.map((preset) => preset.id).join(', ');
    final training = session.presetsInUse
        .where((preset) => preset.trainsOnData)
        .map((preset) => preset.id)
        .join(', ');
    return ListView(
      key: const Key('settings-read-only'),
      padding: const EdgeInsets.all(16),
      children: [
        Text(
          l10n.settingsWebProviders,
          style: Theme.of(context).textTheme.titleMedium,
        ),
        const SizedBox(height: 12),
        _SummaryRow(
          icon: Icons.record_voice_over_outlined,
          label: l10n.settingsWebSpeech,
          value: session.demo ? l10n.providerDemo : l10n.settingsServerDefault,
        ),
        _SummaryRow(
          icon: Icons.translate,
          label: l10n.settingsWebPreset,
          value: presets.isEmpty ? l10n.settingsServerDefault : presets,
        ),
        _SummaryRow(
          icon: Icons.volume_up_outlined,
          label: l10n.settingsWebVoice,
          value: session.tts == 'audio'
              ? l10n.settingsServerDefault
              : l10n.settingsVoiceSystem,
        ),
        if (training.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Row(
              children: [
                Icon(Icons.info_outline, size: 18, color: colors.ink),
                const SizedBox(width: 8),
                Expanded(child: Text(l10n.presetTrainsOnData(training))),
              ],
            ),
          ),
        const SizedBox(height: 24),
        Text(l10n.settingsWebReadOnly, style: TextStyle(color: colors.muted)),
        const SizedBox(height: 8),
        Text(l10n.settingsWebNoKeys, style: TextStyle(color: colors.muted)),
      ],
    );
  }
}

class _SummaryRow extends StatelessWidget {
  const _SummaryRow({
    required this.icon,
    required this.label,
    required this.value,
  });

  final IconData icon;
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          Icon(icon, size: 18, color: colors.muted),
          const SizedBox(width: 8),
          Expanded(child: Text(label)),
          Text(value, style: TextStyle(color: colors.muted)),
        ],
      ),
    );
  }
}

class _Card extends StatelessWidget {
  const _Card({
    required this.icon,
    required this.title,
    required this.children,
  });

  final IconData icon;
  final String title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    final colors = ParleyColors.of(context);
    return Card(
      color: colors.surface,
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(icon, size: 18, color: colors.ink),
                const SizedBox(width: 8),
                Text(title, style: Theme.of(context).textTheme.titleMedium),
              ],
            ),
            const SizedBox(height: 12),
            ...children,
          ],
        ),
      ),
    );
  }
}

class _TextInput extends StatelessWidget {
  const _TextInput({
    super.key,
    required this.label,
    required this.value,
    required this.onChanged,
    this.error,
  });

  final String label;
  final String value;
  final ValueChanged<String> onChanged;
  final String? error;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: TextFormField(
        initialValue: value,
        decoration: InputDecoration(labelText: label, errorText: error),
        onChanged: onChanged,
      ),
    );
  }
}

/// A key is displayed only as bullets plus its last four characters; a change
/// opens an empty field so the old secret is never rendered.
class _SecretField extends StatelessWidget {
  const _SecretField({
    required this.field,
    required this.label,
    required this.value,
    required this.editing,
    required this.onEdit,
    required this.onClear,
    required this.onChanged,
  });

  final String field;
  final String label;
  final String value;
  final bool editing;
  final VoidCallback onEdit;
  final VoidCallback onClear;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final colors = ParleyColors.of(context);
    if (editing) {
      return Padding(
        padding: const EdgeInsets.only(top: 8),
        child: TextFormField(
          key: Key('secret-input-$field'),
          obscureText: true,
          decoration: InputDecoration(labelText: label),
          onChanged: onChanged,
        ),
      );
    }
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Row(
        children: [
          Expanded(
            child: Text(
              value.isEmpty
                  ? l10n.settingsKeyMissing
                  : l10n.settingsKeySaved(maskedSecret(value)),
              key: Key('secret-mask-$field'),
              style: TextStyle(color: colors.muted),
            ),
          ),
          TextButton(onPressed: onEdit, child: Text(l10n.settingsKeyChange)),
          if (value.isNotEmpty)
            TextButton(onPressed: onClear, child: Text(l10n.settingsKeyClear)),
        ],
      ),
    );
  }
}

class _Hint extends StatelessWidget {
  const _Hint(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Text(
        text,
        style: TextStyle(color: ParleyColors.of(context).muted, fontSize: 12),
      ),
    );
  }
}

class _ProviderError extends StatelessWidget {
  const _ProviderError({required this.message});

  final String? message;

  @override
  Widget build(BuildContext context) {
    if (message == null) return const SizedBox.shrink();
    final l10n = AppLocalizations.of(context);
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            Icons.error_outline,
            size: 18,
            color: ParleyColors.of(context).error,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              l10n.settingsLastError(message!),
              key: const Key('provider-error'),
              style: TextStyle(color: ParleyColors.of(context).error),
            ),
          ),
        ],
      ),
    );
  }
}
