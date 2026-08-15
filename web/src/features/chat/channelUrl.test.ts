import { describe, expect, it } from "vitest";

import { channelFromSearch, normalizeChannelName, searchWithChannel } from "./channelUrl";

describe("normalizeChannelName", () => {
  it("accepts a bare keyword and a prefixed one alike", () => {
    expect(normalizeChannelName("general")).toBe("?general");
    expect(normalizeChannelName("?general")).toBe("?general");
    expect(normalizeChannelName("  ?General  ")).toBe("?general");
    expect(normalizeChannelName("dev-ops_2")).toBe("?dev-ops_2");
  });

  it("rejects what the server would reject", () => {
    expect(normalizeChannelName("")).toBeNull();
    expect(normalizeChannelName("?")).toBeNull();
    expect(normalizeChannelName("---")).toBeNull();
    expect(normalizeChannelName("has space")).toBeNull();
    expect(normalizeChannelName("bad#char")).toBeNull();
    expect(normalizeChannelName("x".repeat(33))).toBeNull();
    expect(normalizeChannelName("x".repeat(32))).toBe(`?${"x".repeat(32)}`);
  });
});

describe("channelFromSearch", () => {
  it("reads the keyword out of the query string", () => {
    expect(channelFromSearch("?general")).toBe("?general");
    expect(channelFromSearch("general")).toBe("?general");
    expect(channelFromSearch("?General")).toBe("?general");
    expect(channelFromSearch("?general&utm=x")).toBe("?general");
    expect(channelFromSearch("?utm=x&general")).toBe("?general");
  });

  it("returns null when there is no usable keyword", () => {
    expect(channelFromSearch("")).toBeNull();
    expect(channelFromSearch("?")).toBeNull();
    expect(channelFromSearch("?utm=x")).toBeNull();
    expect(channelFromSearch("?%zz")).toBeNull();
  });
});

describe("searchWithChannel", () => {
  it("writes the keyword as the first query key", () => {
    expect(searchWithChannel("", "?general")).toBe("?general");
    expect(searchWithChannel("?teste", "?general")).toBe("?general");
    expect(searchWithChannel("?teste&utm=x", "?general")).toBe("?general&utm=x");
  });

  it("keeps foreign params that are not a channel keyword", () => {
    expect(searchWithChannel("?utm=x", "?general")).toBe("?general&utm=x");
    expect(searchWithChannel("?utm=x&teste", "?general")).toBe("?utm=x&general");
  });

  it("leaves the search untouched for a name it cannot express", () => {
    expect(searchWithChannel("?teste", "bad name")).toBe("?teste");
  });
});
