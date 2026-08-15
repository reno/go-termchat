import { Line, Pane, Spinner, StatusBar } from "@/components/term";
import { JoinPrompt } from "@/features/session/JoinPrompt";
import { useSession } from "@/features/session/useSession";
import type { SessionClients } from "@/lib/clients";

export interface AppProps {
  clients: SessionClients;
}

export function App({ clients }: AppProps) {
  const { state, join } = useSession(clients);

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

  if (state.phase === "anonymous") {
    return <JoinPrompt onJoin={join} notice={state.notice} busy={state.busy} />;
  }

  return (
    <div className="app">
      <StatusBar label="channel status">
        <span className="app__nick">{state.nick}</span>
        <span className="app__tagline">termchat</span>
      </StatusBar>
      <Pane scroll label="transcript" live>
        <Line nick="*" variant="system">
          joined as {state.nick}. the chat UI lands in M7.
        </Line>
      </Pane>
    </div>
  );
}
