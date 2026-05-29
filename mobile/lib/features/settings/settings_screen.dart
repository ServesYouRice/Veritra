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
          const SwitchListTile(
            value: false,
            onChanged: null,
            secondary: Icon(Icons.notifications_outlined),
            title: Text('Push notifications'),
          ),
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
          const ListTile(
            leading: Icon(Icons.key_outlined),
            title: Text('Recovery'),
          ),
          const ListTile(
            leading: Icon(Icons.video_call_outlined),
            title: Text('Calls'),
          ),
          const ListTile(
            leading: Icon(Icons.privacy_tip_outlined),
            title: Text('Privacy'),
          ),
        ],
      ),
    );
  }
}
