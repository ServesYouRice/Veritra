import 'package:flutter/material.dart';

import '../../core/app_state.dart';
import 'device_link_screen.dart';

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: ListView(
        children: <Widget>[
          ListTile(
            leading: const Icon(Icons.qr_code_2),
            title: const Text('Link device'),
            onTap: () {
              Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) => DeviceLinkScreen(state: state),
                ),
              );
            },
          ),
          // Placeholder entries point at planned screens; do not surface
          // controls that have no implementation behind them yet
          // (avoids implying a feature exists when it does not).
          const ListTile(
            leading: Icon(Icons.key_outlined),
            title: Text('Recovery'),
            subtitle: Text('Encrypted backup & recovery key (coming soon)'),
            enabled: false,
          ),
          const ListTile(
            leading: Icon(Icons.video_call_outlined),
            title: Text('Calls'),
            subtitle: Text('1:1 audio/video (coming soon)'),
            enabled: false,
          ),
          const ListTile(
            leading: Icon(Icons.privacy_tip_outlined),
            title: Text('Privacy'),
            subtitle: Text('Configurable retention & visibility (coming soon)'),
            enabled: false,
          ),
        ],
      ),
    );
  }
}
