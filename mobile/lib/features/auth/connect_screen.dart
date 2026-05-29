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

  Future<void> _submit() async {
    final raw = url.text.trim();
    if (!await _confirmInsecureUrl(raw)) {
      return;
    }
    switch (mode) {
    case AuthMode.owner:
      return widget.state.createOwner(raw, username.text.trim(), password.text);
    case AuthMode.signIn:
      return widget.state.login(raw, username.text.trim(), password.text);
    case AuthMode.linkDevice:
      return widget.state.claimDeviceLink(raw, linkCode.text.trim());
    }
  }

  /// Returns true if the URL is safe to use (HTTPS, or a clearly-local
  /// HTTP target like localhost / 127.0.0.1 / *.local), or if the user
  /// has explicitly confirmed an insecure public URL.
  Future<bool> _confirmInsecureUrl(String raw) async {
    if (raw.isEmpty) {
      return true; // let downstream validation produce a clearer error
    }
    final parsed = Uri.tryParse(raw);
    if (parsed == null || !parsed.hasScheme) {
      return true;
    }
    if (parsed.scheme == 'https') {
      return true;
    }
    if (parsed.scheme != 'http') {
      return true;
    }
    final host = parsed.host.toLowerCase();
    if (_isLocalHost(host)) {
      return true;
    }
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Use insecure connection?'),
        content: Text(
          'You are about to connect to $host over plain HTTP.\n\n'
          'Your password, session token, and message metadata would be sent '
          'in cleartext. Use https:// in production.',
        ),
        actions: <Widget>[
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: const Text('Continue anyway'),
          ),
        ],
      ),
    );
    return confirmed ?? false;
  }

  bool _isLocalHost(String host) {
    if (host == 'localhost' || host == '127.0.0.1' || host == '::1') {
      return true;
    }
    if (host.endsWith('.local') || host.endsWith('.localhost')) {
      return true;
    }
    // RFC 1918 private ranges + loopback. Cheap string-prefix check; if the
    // host is an FQDN that happens to start with "10." we still flag it as
    // local, which is conservative for a dev convenience.
    if (host.startsWith('10.') || host.startsWith('192.168.')) {
      return true;
    }
    if (host.startsWith('172.')) {
      final parts = host.split('.');
      if (parts.length >= 2) {
        final secondOctet = int.tryParse(parts[1]);
        if (secondOctet != null && secondOctet >= 16 && secondOctet <= 31) {
          return true;
        }
      }
    }
    return false;
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
