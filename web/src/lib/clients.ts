import type { Client } from "@connectrpc/connect";

import type { ChatService } from "@gen/chat/v1/chat_pb";
import type { SessionService } from "@gen/chat/v1/session_pb";

export interface SessionClients {
  sessions: Client<typeof SessionService>;
  chat: Client<typeof ChatService>;
}
