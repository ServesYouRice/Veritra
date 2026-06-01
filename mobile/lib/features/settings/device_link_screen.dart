import 'package:flutter/material.dart';

import '../../core/app_state.dart';
import '../../core/models.dart';

class DeviceLinkScreen extends StatelessWidget {
  const DeviceLinkScreen({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: state,
      builder: (context, _) {
        final link = state.activeDeviceLink;
        return Scaffold(
          appBar: AppBar(title: const Text('Link device')),
          body: ListView(
            padding: const EdgeInsets.all(16),
            children: <Widget>[
              FilledButton.icon(
                onPressed: state.busy ? null : state.createDeviceLink,
                icon: const Icon(Icons.qr_code_2),
                label: Text(link == null ? 'Create link' : 'Create new link'),
              ),
              if (link != null) ...<Widget>[
                const SizedBox(height: 16),
                _DeviceLinkDetails(link: link),
                const SizedBox(height: 16),
                OutlinedButton.icon(
                  onPressed: state.busy ? null : state.refreshActiveDeviceLink,
                  icon: const Icon(Icons.refresh),
                  label: const Text('Refresh status'),
                ),
                const SizedBox(height: 8),
                FilledButton.icon(
                  onPressed: state.busy ? null : () => _approve(context),
                  icon: const Icon(Icons.verified_user_outlined),
                  label: const Text('Approve device'),
                ),
              ],
              if (state.error != null) ...<Widget>[
                const SizedBox(height: 12),
                Text(
                  state.error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
            ],
          ),
        );
      },
    );
  }

  Future<void> _approve(BuildContext context) async {
    final controller = TextEditingController();
    try {
      final code = await showDialog<String>(
        context: context,
        builder: (context) {
          return AlertDialog(
            title: const Text('Verification code'),
            content: TextField(
              controller: controller,
              autofocus: true,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(hintText: '000000'),
            ),
            actions: <Widget>[
              TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: const Text('Cancel'),
              ),
              FilledButton(
                onPressed: () => Navigator.of(context).pop(controller.text),
                child: const Text('Approve'),
              ),
            ],
          );
        },
      );
      final trimmed = code?.trim();
      if (trimmed == null || trimmed.isEmpty) {
        return;
      }
      await state.approveActiveDeviceLink(trimmed);
    } finally {
      controller.dispose();
    }
  }
}

class _DeviceLinkDetails extends StatelessWidget {
  const _DeviceLinkDetails({required this.link});

  final DeviceLink link;

  @override
  Widget build(BuildContext context) {
    final linkUri = link.linkUri;
    return Column(
      children: <Widget>[
        _LinkValueTile(
          icon: Icons.pin_outlined,
          title: 'Link code',
          value: link.code ?? '',
        ),
        _LinkValueTile(
          icon: Icons.verified_outlined,
          title: 'Verification code',
          value: link.verificationCode,
        ),
        if (linkUri != null)
          _LinkValueTile(
            icon: Icons.link_outlined,
            title: 'Link URI',
            value: linkUri,
          ),
        _LinkValueTile(
          icon: Icons.timer_outlined,
          title: 'Expires',
          value: link.expiresAt.toLocal().toString(),
        ),
        _LinkValueTile(
          icon: Icons.info_outline,
          title: 'State',
          value: link.state,
        ),
        if (link.claimedDeviceName != null)
          _LinkValueTile(
            icon: Icons.tablet_android_outlined,
            title: 'Claimed device',
            value: link.claimedDeviceName!,
          ),
      ],
    );
  }
}

class _LinkValueTile extends StatelessWidget {
  const _LinkValueTile({
    required this.icon,
    required this.title,
    required this.value,
  });

  final IconData icon;
  final String title;
  final String value;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: Icon(icon),
      title: Text(title),
      subtitle: SelectableText(value),
      contentPadding: EdgeInsets.zero,
    );
  }
}
