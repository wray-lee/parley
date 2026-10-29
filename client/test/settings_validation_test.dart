import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/settings/validation.dart';

/// The fixture is the one case table the Go core reads too.
Map<String, dynamic> loadFixture() => jsonDecode(
  File('../spec/fixtures/settings-validation.json').readAsStringSync(),
) as Map<String, dynamic>;

String errorCode(Object error) =>
    error is SettingsValidationError ? error.code : '$error';

void main() {
  final fixture = loadFixture();

  test('base URL cases match spec/fixtures/settings-validation.json', () {
    for (final testCase in fixture['baseUrl'] as List<dynamic>) {
      final kase = testCase as Map<String, dynamic>;
      final value = kase['value'] as String;
      final preset = kase['preset'] as String? ?? '';
      if (kase['error'] != null) {
        expect(
          () => normalizeBaseUrl(value, preset: preset),
          throwsA(
            predicate<Object>(
              (error) => errorCode(error) == kase['error'],
              'error ${kase['error']}',
            ),
          ),
          reason: '$value ($preset)',
        );
      } else {
        expect(normalizeBaseUrl(value, preset: preset), kase['expect']);
      }
    }
  });

  test('model cases match spec/fixtures/settings-validation.json', () {
    for (final testCase in fixture['model'] as List<dynamic>) {
      final kase = testCase as Map<String, dynamic>;
      final value = kase['value'] as String;
      final preset = kase['preset'] as String? ?? '';
      if (kase['error'] != null) {
        expect(
          () => normalizeModel(value, preset: preset),
          throwsA(
            predicate<Object>(
              (error) => errorCode(error) == kase['error'],
              'error ${kase['error']}',
            ),
          ),
          reason: '$value ($preset)',
        );
      } else {
        expect(normalizeModel(value, preset: preset), kase['expect']);
      }
    }
  });

  test('extra body cases match spec/fixtures/settings-validation.json', () {
    for (final testCase in fixture['extraBody'] as List<dynamic>) {
      final kase = testCase as Map<String, dynamic>;
      final value = kase['value'] as String;
      if (kase['error'] != null) {
        expect(
          () => normalizeExtraBody(value),
          throwsA(
            predicate<Object>(
              (error) => errorCode(error) == kase['error'],
              'error ${kase['error']}',
            ),
          ),
          reason: value,
        );
      } else {
        expect(normalizeExtraBody(value), kase['expect']);
      }
    }
  });

  test('local network cases match spec/fixtures/settings-validation.json', () {
    final section = fixture['localNetwork'] as Map<String, dynamic>;
    final base = Map<String, String>.from(section['base'] as Map);
    for (final testCase in section['cases'] as List<dynamic>) {
      final kase = testCase as Map<String, dynamic>;
      final values = {...base, ...Map<String, String>.from(kase['set'] as Map)};
      expect(
        needsLocalNetworkPermission(values),
        kase['expect'],
        reason: '${kase['set']}',
      );
    }
  });

  test('keys are masked with the last four characters', () {
    expect(maskedSecret(''), '');
    expect(maskedSecret('abc'), '••••');
    expect(maskedSecret('sk-live-1234abcd'), '••••abcd');
    expect(maskedSecret('sk-live-1234abcd'), isNot(contains('1234')));
  });
}
