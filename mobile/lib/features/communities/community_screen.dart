import 'package:flutter/material.dart';

import '../../core/app_state.dart';

class CommunityScreen extends StatelessWidget {
  const CommunityScreen({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Communities')),
      body: ListView(
        children: const <Widget>[
          ListTile(
            leading: Icon(Icons.groups_outlined),
            title: Text('Private groups'),
          ),
          ListTile(
            leading: Icon(Icons.campaign_outlined),
            title: Text('Announcements'),
          ),
        ],
      ),
    );
  }
}
