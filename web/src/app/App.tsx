import { Line, Pane, Spinner } from "@/components/term";
import { Chat } from "@/features/chat/Chat";
import { useChatStore } from "@/features/chat/store";
import { JoinPrompt } from "@/features/session/JoinPrompt";
import { useSession } from "@/features/session/useSession";
import type { SessionClients } from "@/lib/clients";

export interface AppProps {
  clients: SessionClients;
}

export function App({ clients }: AppProps) {
  const { state, join, leave } = useSession(clients);
  const reset = useChatStore((store) => store.reset);

  if (state.phase === "resuming") {
    return (
      <main className="app-booting">
        <Pane label="connecting" live>
          <Line nick="*" variant="system">
            <Spinner label="connecting" /> connecting
          </Line>
        </Pane>
      </main>
    );
  }

  if (state.phase === "anonymous" || state.nick === null) {
    return <JoinPrompt onJoin={join} notice={state.notice} busy={state.busy} />;
  }

  return (
    <Chat
      clients={clients}
      me={state.nick}
      onLeave={() => {
        reset();
        void leave();
      }}
    />
  );
}
