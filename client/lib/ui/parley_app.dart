import 'package:material_ui/material_ui.dart';

import '../session/listen_session.dart';
import '../theme.dart';
import 'listen_screen.dart';

class ParleyApp extends StatefulWidget {
  const ParleyApp({super.key, required this.createSession});

  final ListenSession Function() createSession;

  @override
  State<ParleyApp> createState() => _ParleyAppState();
}

class _ParleyAppState extends State<ParleyApp> {
  late final ListenSession _session;

  @override
  void initState() {
    super.initState();
    _session = widget.createSession();
    _session.connect();
  }

  @override
  void dispose() {
    _session.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Parley',
      debugShowCheckedModeBanner: false,
      theme: parleyTheme(Brightness.light),
      darkTheme: parleyTheme(Brightness.dark),
      themeMode: ThemeMode.dark,
      home: ListenScreen(session: _session),
    );
  }
}
