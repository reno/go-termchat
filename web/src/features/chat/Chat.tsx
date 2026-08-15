import { useEffect, useRef, useState } from "react";

import type { SessionClients } from "@/lib/clients";

import { Composer } from "./Composer";
import { Header } from "./Header";
import { HintBar } from "./HintBar";
import { SidePanel } from "./SidePanel";
import { Transcript } from "./Transcript";
import { useChat } from "./useChat";
import { useFocusTrap } from "./useFocusTrap";
import { useNarrow } from "./useNarrow";
import { useChatStore } from "./store";

import "./chat.css";

export interface ChatProps {
  clients: SessionClients;
  me: string;
  onLeave: () => void;
}

export function Chat({ clients, me, onLeave }: ChatProps) {
  const narrow = useNarrow();
  const [panelOpen, setPanelOpen] = useState(false);
  const [wasNarrow, setWasNarrow] = useState(narrow);
  const panelRef = useRef<HTMLDivElement>(null);

  const channels = useChatStore((state) => state.channels);
  const activeId = useChatStore((state) => state.activeId);
  const messages = useChatStore((state) => state.messages);
  const members = useChatStore((state) => state.members);

  const { draft, setDraft, send, mention, selectChannel, setTyping, loadOlder } = useChat(
    clients,
    me,
    onLeave,
  );

  if (wasNarrow !== narrow) {
    setWasNarrow(narrow);
    setPanelOpen(false);
  }

  useFocusTrap(panelRef, narrow && panelOpen);

  const active = channels.find((channel) => channel.id === activeId) ?? null;
  const roster = activeId === null ? [] : (members[activeId] ?? []);
  const lines = activeId === null ? [] : (messages[activeId] ?? []);
  const online = roster.filter((member) => member.nick !== me).length + 1;

  useEffect(() => {
    if (!panelOpen || !narrow) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setPanelOpen(false);
      }
    };
    globalThis.addEventListener("keydown", onKeyDown);
    return () => {
      globalThis.removeEventListener("keydown", onKeyDown);
    };
  }, [panelOpen, narrow]);

  const chooseChannel = (channelId: string) => {
    selectChannel(channelId);
    setPanelOpen(false);
  };

  return (
    <div className="chat" data-narrow={narrow}>
      <Header
        channel={active?.name ?? ""}
        online={online}
        narrow={narrow}
        panelOpen={panelOpen}
        onTogglePanel={() => {
          setPanelOpen((open) => !open);
        }}
      />

      <div className="chat__body">
        <div className="chat__column">
          <Transcript
            messages={lines}
            me={me}
            onReachTop={() => {
              void loadOlder();
            }}
          />
          {!narrow && <HintBar />}
          <Composer
            draft={draft}
            onDraft={setDraft}
            onSend={() => {
              void send();
            }}
            onTyping={setTyping}
            narrow={narrow}
          />
        </div>

        {narrow && panelOpen && (
          <div
            className="chat__scrim"
            onClick={() => {
              setPanelOpen(false);
            }}
            aria-hidden="true"
          />
        )}

        <div
          ref={panelRef}
          className="chat__panel-slot"
          data-open={!narrow || panelOpen}
          hidden={narrow && !panelOpen}
        >
          <SidePanel
            channels={channels}
            activeId={activeId}
            members={roster}
            me={me}
            onSelectChannel={chooseChannel}
            onMention={mention}
          />
        </div>
      </div>
    </div>
  );
}
