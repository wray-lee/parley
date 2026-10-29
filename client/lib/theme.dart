import 'package:material_ui/material_ui.dart';

/// The Caption theme's ink/paper tokens, dark-first. Tally red means "live"
/// and nothing else (`.scratch/v01-research/SYNTHESIS.md` §1.3).
@immutable
class ParleyColors {
  const ParleyColors({
    required this.bg,
    required this.surface,
    required this.control,
    required this.ink,
    required this.muted,
    required this.provisional,
    required this.outline,
    required this.hairline,
    required this.live,
    required this.onLive,
    required this.liveText,
    required this.demoForeground,
    required this.demoBackground,
    required this.error,
  });

  static const ParleyColors dark = ParleyColors(
    bg: Color(0xFF0C0C0D),
    surface: Color(0xFF151516),
    control: Color(0xFF1E1E20),
    ink: Color(0xFFF2F1ED),
    muted: Color(0xFFA2A2A7),
    provisional: Color(0xFFC8C7C2),
    outline: Color(0xFF68686E),
    hairline: Color(0xFF28282B),
    live: Color(0xFFFF5B49),
    onLive: Color(0xFF0C0C0D),
    liveText: Color(0xFFFF9186),
    demoForeground: Color(0xFFEAC17B),
    demoBackground: Color(0xFF2A2214),
    error: Color(0xFFFFB4AB),
  );

  static const ParleyColors light = ParleyColors(
    bg: Color(0xFFF2F2EF),
    surface: Color(0xFFFFFFFF),
    control: Color(0xFFE6E6E1),
    ink: Color(0xFF0C0C0D),
    muted: Color(0xFF5C5C60),
    provisional: Color(0xFF3C3C40),
    outline: Color(0xFF80807C),
    hairline: Color(0xFFD6D6D0),
    live: Color(0xFFC92C1E),
    onLive: Color(0xFFFFFFFF),
    liveText: Color(0xFFA21E13),
    demoForeground: Color(0xFF634200),
    demoBackground: Color(0xFFF4E4C6),
    error: Color(0xFFBA1A1A),
  );

  static ParleyColors of(BuildContext context) =>
      Theme.of(context).brightness == Brightness.dark ? dark : light;

  final Color bg;
  final Color surface;
  final Color control;
  final Color ink;
  final Color muted;
  final Color provisional;
  final Color outline;
  final Color hairline;
  final Color live;
  final Color onLive;
  final Color liveText;
  final Color demoForeground;
  final Color demoBackground;
  final Color error;
}

ThemeData parleyTheme(Brightness brightness) {
  final colors = brightness == Brightness.dark
      ? ParleyColors.dark
      : ParleyColors.light;
  final scheme = ColorScheme(
    brightness: brightness,
    primary: colors.ink,
    onPrimary: colors.bg,
    secondary: colors.live,
    onSecondary: colors.onLive,
    error: colors.error,
    onError: colors.bg,
    surface: colors.surface,
    onSurface: colors.ink,
    onSurfaceVariant: colors.muted,
    outline: colors.outline,
    outlineVariant: colors.hairline,
    surfaceContainer: colors.control,
    surfaceContainerHighest: colors.control,
  );
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: colors.bg,
  );
}
