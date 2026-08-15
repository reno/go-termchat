import { Badge } from "@/components/term";

import type { ChannelView, MemberView } from "./model";
import { nickColor } from "./model";

export interface SidePanelProps {
  channels: ChannelView[];
  activeId: string | null;
  members: MemberView[];
  me: string | null;
  onSelectChannel: (channelId: string) => void;
  onMention: (nick: string) => void;
}

export const PANEL_FOOTER = "type /help for commands";

export function SidePanel({
  channels,
  activeId,
  members,
  me,
  onSelectChannel,
  onMention,
}: SidePanelProps) {
  const roster: MemberView[] = [
    ...members.filter((member) => member.nick !== me),
    ...(me === null ? [] : [{ nick: me, tag: "me" }]),
  ];

  return (
    <aside className="chat__panel" id="chat-panel" aria-label="channels and roster">
      <h2 className="chat__label">CHANNELS</h2>
      <ul className="chat__channels">
        {channels.map((channel) => {
          const active = channel.id === activeId;
          return (
            <li key={channel.id}>
              <button
                type="button"
                className="chat__channel-row"
                data-active={active}
                aria-current={active ? "true" : undefined}
                onClick={() => {
                  onSelectChannel(channel.id);
                }}
              >
                <span className="chat__channel-name">{channel.name}</span>
                <Badge count={channel.memberCount} onRaised={active} />
              </button>
            </li>
          );
        })}
      </ul>

      <h2 className="chat__label chat__label--online">ONLINE</h2>
      <ul className="chat__roster">
        {roster.map((member) => (
          <li key={member.nick}>
            <button
              type="button"
              className="chat__roster-entry"
              onClick={() => {
                onMention(member.nick);
              }}
            >
              <span className="chat__roster-nick" style={{ color: nickColor(member.nick, me) }}>
                {member.nick}
              </span>
              {member.tag !== "" && <Badge>{member.tag}</Badge>}
            </button>
          </li>
        ))}
      </ul>

      <p className="chat__panel-footer">{PANEL_FOOTER}</p>
    </aside>
  );
}
