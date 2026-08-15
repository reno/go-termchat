const CHANNEL_PREFIX = "?";
const NAME_MAX_LEN = 32;
const ALLOWED = /^[a-z0-9_-]+$/;
const ALPHANUMERIC = /[a-z0-9]/;

export function normalizeChannelName(raw: string): string | null {
  let name = raw.trim().toLowerCase();
  if (name.startsWith(CHANNEL_PREFIX)) {
    name = name.slice(CHANNEL_PREFIX.length);
  }
  if (name === "" || name.length > NAME_MAX_LEN) {
    return null;
  }
  if (!ALLOWED.test(name) || !ALPHANUMERIC.test(name)) {
    return null;
  }
  return CHANNEL_PREFIX + name;
}

function parts(search: string): string[] {
  const raw = search.startsWith(CHANNEL_PREFIX) ? search.slice(1) : search;
  return raw.split("&").filter((part) => part !== "");
}

function asChannel(part: string): string | null {
  if (part.includes("=")) {
    return null;
  }
  try {
    return normalizeChannelName(decodeURIComponent(part));
  } catch {
    return null;
  }
}

export function channelFromSearch(search: string): string | null {
  for (const part of parts(search)) {
    const name = asChannel(part);
    if (name !== null) {
      return name;
    }
  }
  return null;
}

export function searchWithChannel(search: string, channelName: string): string {
  const name = normalizeChannelName(channelName);
  if (name === null) {
    return search;
  }
  const keyword = encodeURIComponent(name.slice(1));
  const existing = parts(search);
  const at = existing.findIndex((part) => asChannel(part) !== null);
  const next = at === -1 ? [keyword, ...existing] : existing.with(at, keyword);
  return CHANNEL_PREFIX + next.join("&");
}
