import 'package:flutter/material.dart';

import '../../core/app_state.dart';

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: ListView(
        children: const <Widget>[
          SwitchListTile(value: false, onChanged: null, secondary: Icon(Icons.notifications_outlined), title: Text('Push notifications')),
          ListTile(leading: Icon(Icons.qr_code_2), title: Text('Link device')),
          ListTile(leading: Icon(Icons.key_outlined), title: Text('Recovery')),
          ListTile(leading: Icon(Icons.video_call_outlined), title: Text('Calls')),
          ListTile(leading: Icon(Icons.privacy_tip_outlined), title: Text('Privacy')),
        ],
      ),
    );
  }
}

