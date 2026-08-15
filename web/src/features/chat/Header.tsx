export interface HeaderProps {
  channel: string;
  online: number;
  narrow: boolean;
  panelOpen: boolean;
  onTogglePanel: () => void;
}

export const TAGLINE = "termchat";

export function Header({ channel, online, narrow, panelOpen, onTogglePanel }: HeaderProps) {
  return (
    <header className="chat__header">
      {narrow && (
        <button
          type="button"
          className="chat__menu"
          onClick={onTogglePanel}
          aria-expanded={panelOpen}
          aria-controls="chat-panel"
          aria-label="channels and roster"
        >
          ☰
        </button>
      )}
      <span className="chat__channel">{channel}</span>
      <span className="chat__sep" aria-hidden="true">
        ·
      </span>
      <span className="chat__online">{online} online</span>
      {!narrow && <span className="chat__tagline">{TAGLINE}</span>}
    </header>
  );
}
