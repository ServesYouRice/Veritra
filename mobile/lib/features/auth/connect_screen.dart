import 'package:flutter/material.dart';

import '../../core/app_state.dart';

enum AuthMode { owner, signIn, linkDevice }

class ConnectScreen extends StatefulWidget {
  const ConnectScreen({required this.state, super.key});

  final AppState state;

  @override
  State<ConnectScreen> createState() => _ConnectScreenState();
}

class _ConnectScreenState extends State<ConnectScreen> {
  final url = TextEditingController(text: 'http://localhost:8080');
  final username = TextEditingController();
  final password = TextEditingController();
  final linkCode = TextEditingController();
  AuthMode mode = AuthMode.owner;

  @override
  void dispose() {
    url.dispose();
    username.dispose();
    password.dispose();
    linkCode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final pendingLink = widget.state.pendingDeviceLinkClaim?.deviceLink;
    return Scaffold(
      appBar: AppBar(title: const Text('Private Messenger')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: <Widget>[
          SegmentedButton<AuthMode>(
            segments: const <ButtonSegment<AuthMode>>[
              ButtonSegment<AuthMode>(
                value: AuthMode.owner,
                label: Text('Owner'),
              ),
              ButtonSegment<AuthMode>(
                value: AuthMode.signIn,
                label: Text('Sign in'),
              ),
              ButtonSegment<AuthMode>(
                value: AuthMode.linkDevice,
                label: Text('Link'),
              ),
            ],
            selected: <AuthMode>{mode},
            onSelectionChanged: (value) => setState(() => mode = value.first),
          ),
          const SizedBox(height: 16),
          TextField(
            controller: url,
            decoration: const InputDecoration(
              labelText: 'Instance URL',
              prefixIcon: Icon(Icons.dns_outlined),
            ),
          ),
          const SizedBox(height: 12),
          if (mode == AuthMode.linkDevice) ...<Widget>[
            TextField(
              controller: linkCode,
              decoration: const InputDecoration(
                labelText: 'Link code',
                prefixIcon: Icon(Icons.qr_code_2),
              ),
            ),
            if (pendingLink != null) ...<Widget>[
              const SizedBox(height: 12),
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.verified_outlined),
                title: const Text('Verification code'),
                subtitle: SelectableText(pendingLink.verificationCode),
              ),
            ],
          ] else ...<Widget>[
            TextField(
              controller: username,
              decoration: const InputDecoration(
                labelText: 'Username',
                prefixIcon: Icon(Icons.person_outline),
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: password,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: 'Password',
                prefixIcon: Icon(Icons.lock_outline),
              ),
            ),
          ],
          const SizedBox(height: 16),
          if (mode == AuthMode.linkDevice && pendingLink != null)
            FilledButton.icon(
              onPressed: widget.state.busy ? null : _completeDeviceLink,
              icon: const Icon(Icons.sync),
              label: const Text('Check approval'),
            )
          else
            FilledButton.icon(
              onPressed: widget.state.busy ? null : _submit,
              icon: Icon(mode == AuthMode.linkDevice
                  ? Icons.qr_code_2
                  : Icons.login),
              label: Text(_submitLabel),
            ),
          if (widget.state.error != null) ...<Widget>[
            const SizedBox(height: 12),
            Text(widget.state.error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
          ],
        ],
      ),
    );
  }

  Future<void> _submit() {
    switch (mode) {
    case AuthMode.owner:
      return widget.state.createOwner(url.text.trim(), username.text.trim(), password.text);
    case AuthMode.signIn:
      return widget.state.login(url.text.trim(), username.text.trim(), password.text);
    case AuthMode.linkDevice:
      return widget.state.claimDeviceLink(url.text.trim(), linkCode.text.trim());
    }
  }

  Future<void> _completeDeviceLink() {
    return widget.state.completeDeviceLinkClaim();
  }

  String get _submitLabel {
    switch (mode) {
    case AuthMode.owner:
      return 'Create owner';
    case AuthMode.signIn:
      return 'Sign in';
    case AuthMode.linkDevice:
      return 'Claim link';
    }
  }
}
