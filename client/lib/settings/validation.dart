/// Settings validation shared with the Go core through
/// `spec/fixtures/settings-validation.json`. The error codes are the fixture's
/// `error` strings; each host maps them to its own message.
library;

import 'dart:convert';

class SettingsValidationError implements Exception {
  const SettingsValidationError(this.code);

  /// One of `needsBaseUrl`, `invalidUrl`, `notHttp`, `plainHttp`,
  /// `credentials`, `query`, `needsModel`, `invalidJson`, `notObject`.
  final String code;

  @override
  String toString() => 'SettingsValidationError($code)';
}

final RegExp _trailingSlashes = RegExp(r'/+$');
final RegExp _ipv4 = RegExp(r'^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$');
final RegExp _ipv6Local = RegExp(r'^(fe[89ab][0-9a-f]|f[cd][0-9a-f]{2})$');

/// Trims a Base URL and applies the v0.0.2 rules: https anywhere, plain http
/// only on localhost or a private LAN IPv4 address, no credentials, query or
/// fragment. An empty value is allowed unless the custom Preset is selected.
String normalizeBaseUrl(String? value, {String preset = ''}) {
  final baseUrl = (value ?? '').trim().replaceFirst(_trailingSlashes, '');
  if (baseUrl.isEmpty) {
    if (preset == 'custom') throw const SettingsValidationError('needsBaseUrl');
    return '';
  }
  final parsed = Uri.tryParse(baseUrl);
  if (parsed == null || parsed.scheme.isEmpty || parsed.host.isEmpty) {
    throw const SettingsValidationError('invalidUrl');
  }
  if (parsed.scheme != 'http' && parsed.scheme != 'https') {
    throw const SettingsValidationError('notHttp');
  }
  if (parsed.userInfo.isNotEmpty) {
    throw const SettingsValidationError('credentials');
  }
  if (baseUrl.contains('?') || baseUrl.contains('#')) {
    throw const SettingsValidationError('query');
  }
  if (parsed.scheme == 'http' && !_isPrivateHttpHost(parsed.host)) {
    throw const SettingsValidationError('plainHttp');
  }
  return baseUrl;
}

/// Trims a model name. The custom Preset needs one.
String normalizeModel(String? value, {String preset = ''}) {
  final model = (value ?? '').trim();
  if (model.isEmpty && preset == 'custom') {
    throw const SettingsValidationError('needsModel');
  }
  return model;
}

/// Trims optional extra request fields and requires a JSON object when
/// non-empty.
String normalizeExtraBody(String? value) {
  final text = (value ?? '').trim();
  if (text.isEmpty) return '';
  Object? parsed;
  try {
    parsed = jsonDecode(text);
  } on FormatException {
    throw const SettingsValidationError('invalidJson');
  }
  if (parsed is! Map) throw const SettingsValidationError('notObject');
  return text;
}

/// Mirrors the custom Preset check in `normalizeBaseUrl`.
String maskedSecret(String? value) {
  final secret = value ?? '';
  if (secret.isEmpty) return '';
  if (secret.length <= 4) return '••••';
  return '••••${secret.substring(secret.length - 4)}';
}

/// Reports whether saving or connecting would reach a Provider over the LAN,
/// which Android 17 gates behind a runtime permission.
bool needsLocalNetworkPermission(Map<String, String> values) {
  if (values['mode'] == 'server') {
    return _isLocalNetworkUrl(values['serverUrl'] ?? '');
  }
  if (_isLocalNetworkUrl(values['translatorBaseUrl'] ?? '')) return true;
  return values['ttsProvider'] == 'kokoro' &&
      _isLocalNetworkUrl(values['kokoroUrl'] ?? '');
}

bool _isPrivateHttpHost(String hostname) {
  if (hostname == 'localhost') return true;
  final match = _ipv4.firstMatch(hostname);
  if (match == null) return false;
  final octets = [for (var i = 1; i <= 4; i++) int.parse(match.group(i)!)];
  if (octets.any((octet) => octet > 255)) return false;
  final a = octets[0], b = octets[1];
  return a == 10 ||
      a == 127 ||
      (a == 169 && b == 254) ||
      (a == 192 && b == 168) ||
      (a == 172 && b >= 16 && b <= 31);
}

bool _isLocalNetworkUrl(String value) {
  final uri = Uri.tryParse(value.trim());
  if (uri == null || uri.host.isEmpty) return false;
  final host = uri.host.replaceAll('[', '').replaceAll(']', '').toLowerCase();
  if (host == 'localhost' || host == '::1' || host.startsWith('127.')) {
    return false;
  }
  if (host.endsWith('.local') ||
      host.endsWith('.lan') ||
      host.endsWith('.home.arpa')) {
    return true;
  }
  if (_isPrivateHttpHost(host)) return true;
  final first = host.split(':').first;
  if (_ipv6Local.hasMatch(first)) return true;
  if (_ipv4.firstMatch(host) != null) {
    final octets = host.split('.').map(int.parse).toList();
    if (octets[0] == 100 && octets[1] >= 64 && octets[1] <= 127) return true;
  }
  return false;
}
