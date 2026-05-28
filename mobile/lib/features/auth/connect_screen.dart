import 'package:flutter/material.dart';

import '../../core/app_state.dart';

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
  bool ownerMode = true;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Private Messenger')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: <Widget>[
          SegmentedButton<bool>(
            segments: const <ButtonSegment<bool>>[
              ButtonSegment<bool>(value: true, label: Text('Owner')),
              ButtonSegment<bool>(value: false, label: Text('Sign in')),
            ],
            selected: <bool>{ownerMode},
            onSelectionChanged: (value) => setState(() => ownerMode = value.first),
          ),
          const SizedBox(height: 16),
          TextField(controller: url, decoration: const InputDecoration(labelText: 'Instance URL', prefixIcon: Icon(Icons.dns_outlined))),
          const SizedBox(height: 12),
          TextField(controller: username, decoration: const InputDecoration(labelText: 'Username', prefixIcon: Icon(Icons.person_outline))),
          const SizedBox(height: 12),
          TextField(controller: password, obscureText: true, decoration: const InputDecoration(labelText: 'Password', prefixIcon: Icon(Icons.lock_outline))),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: widget.state.busy ? null : _submit,
            icon: const Icon(Icons.login),
            label: Text(ownerMode ? 'Create owner' : 'Sign in'),
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
    if (ownerMode) {
      return widget.state.createOwner(url.text.trim(), username.text.trim(), password.text);
    }
    return widget.state.login(url.text.trim(), username.text.trim(), password.text);
  }
}

