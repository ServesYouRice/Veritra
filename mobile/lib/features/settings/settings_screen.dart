import 'package:flutter/material.dart';

import '../../core/app_state.dart';
import 'device_link_screen.dart';

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: state,
      builder: (context, _) {
        return Scaffold(
          appBar: AppBar(
            title: const Text('Settings'),
            actions: <Widget>[
              IconButton(
                tooltip: 'Refresh',
                onPressed: state.busy ? null : state.refreshDevices,
                icon: const Icon(Icons.refresh),
              ),
            ],
          ),
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
              ListTile(
                leading: const Icon(Icons.logout),
                title: const Text('Sign out'),
                onTap: state.busy ? null : state.logout,
              ),
              ListTile(
                leading: const Icon(Icons.phonelink_erase_outlined),
                title: const Text('Sign out other devices'),
                onTap: state.busy ? null : state.logoutOtherDevices,
              ),
              const Divider(height: 1),
              for (final device in state.devices)
                ListTile(
                  leading: Icon(device.id == state.session?.deviceId
                      ? Icons.phone_android
                      : Icons.devices_other),
                  title: Text(device.revokedAt == null
                      ? device.name
                      : '${device.name} (revoked)'),
                  subtitle: Text(device.id),
                  trailing: IconButton(
                    tooltip: 'Revoke',
                    onPressed: state.busy || device.revokedAt != null
                        ? null
                        : () => state.revokeDevice(device.id),
                    icon: const Icon(Icons.block),
                  ),
                ),
              const Divider(height: 1),
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
                subtitle:
                    Text('Configurable retention & visibility (coming soon)'),
                enabled: false,
              ),
            ],
          ),
        );
      },
    );
  }
}
