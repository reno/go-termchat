import { HINTS } from "./commands";

export function HintBar() {
  return (
    <div className="chat__hints" aria-label="commands">
      {HINTS.map((hint) => (
        <span key={hint.cmd} className="chat__hint">
          <span className="chat__hint-cmd">{hint.cmd}</span>
          {hint.desc !== "" && <span className="chat__hint-desc"> {hint.desc}</span>}
        </span>
      ))}
    </div>
  );
}
