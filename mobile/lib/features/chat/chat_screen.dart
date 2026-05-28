import 'package:flutter/material.dart';

import '../../core/app_state.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({required this.state, super.key});

  final AppState state;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final composer = TextEditingController();

  @override
  Widget build(BuildContext context) {
    final conversation = widget.state.selectedConversation;
    return Scaffold(
      appBar: AppBar(title: Text(conversation?.title ?? 'Thread')),
      body: Column(
        children: <Widget>[
          Expanded(
            child: Center(
              child: Icon(
                conversation == null ? Icons.forum_outlined : Icons.lock_outline,
                size: 48,
                color: Theme.of(context).colorScheme.primary,
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
            child: Row(
              children: <Widget>[
                IconButton(onPressed: conversation == null ? null : () {}, icon: const Icon(Icons.attach_file), tooltip: 'Attach'),
                Expanded(
                  child: TextField(
                    controller: composer,
                    minLines: 1,
                    maxLines: 4,
                    decoration: const InputDecoration(hintText: 'Message'),
                  ),
                ),
                IconButton(
                  onPressed: conversation == null ? null : () => widget.state.sendMessage(composer.text),
                  icon: const Icon(Icons.send),
                  tooltip: 'Send',
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

