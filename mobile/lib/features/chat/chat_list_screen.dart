import 'package:flutter/material.dart';

import '../../core/app_state.dart';

class ChatListScreen extends StatelessWidget {
  const ChatListScreen({required this.state, super.key});

  final AppState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Chats'),
        actions: <Widget>[
          IconButton(
            tooltip: 'Refresh',
            onPressed: state.refreshConversations,
            icon: const Icon(Icons.refresh),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton(
        onPressed: state.createGroup,
        child: const Icon(Icons.add_comment_outlined),
      ),
      body: ListView.separated(
        itemCount: state.conversations.length,
        separatorBuilder: (_, __) => const Divider(height: 1),
        itemBuilder: (context, index) {
          final conversation = state.conversations[index];
          return ListTile(
            leading: const CircleAvatar(child: Icon(Icons.lock_outline)),
            title: Text(conversation.title ?? conversation.kind),
            subtitle: Text(conversation.id),
            selected: state.selectedConversationId == conversation.id,
            onTap: () => state.selectConversation(conversation.id),
          );
        },
      ),
    );
  }
}

