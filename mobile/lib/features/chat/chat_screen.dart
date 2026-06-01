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
  void dispose() {
    composer.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final conversation = widget.state.selectedConversation;
    final messages = widget.state.selectedMessages;
    return Scaffold(
      appBar: AppBar(title: Text(conversation?.title ?? 'Thread')),
      body: Column(
        children: <Widget>[
          Expanded(
            child: conversation == null
                ? _EmptyThreadIcon(icon: Icons.forum_outlined)
                : messages.isEmpty
                    ? _EmptyThreadIcon(icon: Icons.lock_outline)
                    : ListView.builder(
                        reverse: true,
                        padding: const EdgeInsets.all(12),
                        itemCount: messages.length,
                        itemBuilder: (context, index) {
                          final message = messages[index];
                          final mine = message.senderAccountId ==
                              widget.state.session?.accountId;
                          return Align(
                            alignment: mine
                                ? Alignment.centerRight
                                : Alignment.centerLeft,
                            child: ConstrainedBox(
                              constraints: const BoxConstraints(maxWidth: 420),
                              child: Card(
                                shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(8),
                                ),
                                child: ListTile(
                                  leading: const Icon(Icons.lock_outline),
                                  title: Text(message.cryptoProtocol),
                                  subtitle: Text(
                                    '${message.createdAt.toLocal()}\n${message.id}',
                                  ),
                                  dense: true,
                                ),
                              ),
                            ),
                          );
                        },
                      ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
            child: Row(
              children: <Widget>[
                IconButton(
                  onPressed: conversation == null ? null : () {},
                  icon: const Icon(Icons.attach_file),
                  tooltip: 'Attach',
                ),
                Expanded(
                  child: TextField(
                    controller: composer,
                    minLines: 1,
                    maxLines: 4,
                    decoration: const InputDecoration(hintText: 'Message'),
                  ),
                ),
                IconButton(
                  onPressed: conversation == null
                      ? null
                      : () async {
                          final text = composer.text.trim();
                          if (text.isEmpty) {
                            return;
                          }
                          await widget.state.sendMessage(text);
                          if (mounted && widget.state.error == null) {
                            composer.clear();
                          }
                        },
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

class _EmptyThreadIcon extends StatelessWidget {
  const _EmptyThreadIcon({required this.icon});

  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Icon(
        icon,
        size: 48,
        color: Theme.of(context).colorScheme.primary,
      ),
    );
  }
}
