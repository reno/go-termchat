const CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
const TIME_CHARS = 10;
const RANDOM_CHARS = 16;

function encodeTime(now: number): string {
  let remaining = now;
  let out = "";
  for (let i = 0; i < TIME_CHARS; i++) {
    out = (CROCKFORD[remaining % 32] ?? "0") + out;
    remaining = Math.floor(remaining / 32);
  }
  return out;
}

function encodeRandom(): string {
  const bytes = new Uint8Array(RANDOM_CHARS);
  globalThis.crypto.getRandomValues(bytes);

  let out = "";
  for (const byte of bytes) {
    out += CROCKFORD[byte % 32] ?? "0";
  }
  return out;
}

export function newClientId(): string {
  return encodeTime(Date.now()) + encodeRandom();
}
