import { useEffect, useRef } from "react";

import { Line } from "@/components/term";

import type { MessageView } from "./model";
import { renderLine } from "./model";

const NEAR_BOTTOM = 48;

export interface TranscriptProps {
  messages: MessageView[];
  me: string | null;
  onReachTop?: () => void;
}

export function Transcript({ messages, me, onReachTop }: TranscriptProps) {
  const pane = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const previousHeight = useRef(0);

  useEffect(() => {
    const element = pane.current;
    if (element === null) {
      return;
    }
    if (pinned.current) {
      element.scrollTop = element.scrollHeight;
    }
    previousHeight.current = element.scrollHeight;
  }, [messages]);

  const handleScroll = () => {
    const element = pane.current;
    if (element === null) {
      return;
    }
    const distance = element.scrollHeight - element.scrollTop - element.clientHeight;
    pinned.current = distance <= NEAR_BOTTOM;

    if (element.scrollTop === 0 && onReachTop !== undefined) {
      onReachTop();
    }
  };

  return (
    <div
      ref={pane}
      className="chat__transcript"
      onScroll={handleScroll}
      role="log"
      aria-live="polite"
      aria-label="transcript"
      tabIndex={0}
    >
      {messages.map((message) => {
        const line = renderLine(message, me);
        return (
          <Line
            key={message.id === "" ? message.clientId : message.id}
            nick={line.nick}
            variant={line.variant}
            state={message.state}
            nickColor={line.nickColor}
          >
            {line.body}
          </Line>
        );
      })}
    </div>
  );
}
