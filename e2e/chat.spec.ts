import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

async function join(page: Page, nick: string): Promise<void> {
  await page.goto("/");
  await page.getByLabel("nick").fill(nick);
  await page.getByRole("button", { name: "join" }).click();
  await expect(page.getByRole("log", { name: "transcript" })).toBeVisible();
}

async function say(page: Page, body: string): Promise<void> {
  const composer = page.getByLabel("message");
  await composer.fill(body);
  await composer.press("Enter");
}

test("two browsers exchange messages in real time", async ({ browser }) => {
  const first = await browser.newContext();
  const second = await browser.newContext();
  const rill = await first.newPage();
  const other = await second.newPage();

  await join(rill, "rill");
  await join(other, "0x9c");

  await say(rill, "anyone here actually testing");
  await expect(other.getByText("anyone here actually testing")).toBeVisible();

  await say(other, "the void, mostly");
  await expect(rill.getByText("the void, mostly")).toBeVisible();

  await first.close();
  await second.close();
});

test("a direct message reaches only the two parties", async ({ browser }) => {
  const contexts = await Promise.all([
    browser.newContext(),
    browser.newContext(),
    browser.newContext(),
  ]);
  const [sender, recipient, bystander] = await Promise.all(
    contexts.map((context) => context.newPage()),
  );

  await join(sender, "dmfrom");
  await join(recipient, "dmto");
  await join(bystander, "dmbys");

  await say(sender, "/msg dmto for your eyes only");

  await expect(recipient.getByText("for your eyes only")).toBeVisible();
  await expect(sender.getByText("for your eyes only")).toBeVisible();
  await expect(bystander.getByText("for your eyes only")).toBeHidden();

  await Promise.all(contexts.map((context) => context.close()));
});

test("a taken nick is refused inline", async ({ browser }) => {
  const first = await browser.newContext();
  const second = await browser.newContext();
  const holder = await first.newPage();
  const contender = await second.newPage();

  await join(holder, "taken");

  await contender.goto("/");
  await contender.getByLabel("nick").fill("taken");
  await contender.getByRole("button", { name: "join" }).click();

  await expect(contender.getByRole("alert")).toContainText("taken");
  await expect(contender.getByLabel("nick")).toBeVisible();

  await first.close();
  await second.close();
});

test("a session survives a reload", async ({ page }) => {
  await join(page, "reloader");
  await say(page, "before the reload");
  await expect(page.getByText("before the reload")).toBeVisible();

  await page.reload();

  await expect(page.getByRole("log", { name: "transcript" })).toBeVisible();
  await expect(page.getByLabel("nick")).toBeHidden();
  await expect(page.getByText("before the reload")).toBeVisible();
});

test("a newcomer sees only the retained window, while the author keeps its own cache", async ({
  browser,
}) => {
  const authorContext = await browser.newContext();
  const author = await authorContext.newPage();

  await join(author, "pager");

  for (let i = 1; i <= 14; i++) {
    await say(author, `line-${String(i)}`);
    await expect(author.getByText(`line-${String(i)}`, { exact: true })).toBeVisible();
  }

  // The server keeps 10 messages per channel in this run, so the oldest are gone from it.
  const newcomerContext = await browser.newContext();
  const newcomer = await newcomerContext.newPage();
  await join(newcomer, "newcomer");

  await expect(newcomer.getByText("line-14", { exact: true })).toBeVisible();
  await expect(newcomer.getByText("line-1", { exact: true })).toBeHidden();

  // The author's own device cache holds 200 per channel, which is what makes offline
  // reading possible; it legitimately outlives the server's retained window.
  await author.reload();
  await expect(author.getByRole("log", { name: "transcript" })).toBeVisible();
  await expect(author.getByText("line-1", { exact: true })).toBeVisible();

  await authorContext.close();
  await newcomerContext.close();
});

test("messages sent offline are queued and delivered on reconnect", async ({ browser }) => {
  const senderContext = await browser.newContext();
  const watcherContext = await browser.newContext();
  const sender = await senderContext.newPage();
  const watcher = await watcherContext.newPage();

  await join(sender, "outboxer");
  await join(watcher, "watcher");

  await sender.context().setOffline(true);

  await say(sender, "queued-one");
  await say(sender, "queued-two");
  await say(sender, "queued-three");

  await expect(sender.getByText("[3 queued]")).toBeVisible();
  await expect(watcher.getByText("queued-one")).toBeHidden();

  await sender.context().setOffline(false);

  await expect(watcher.getByText("queued-one")).toBeVisible();
  await expect(watcher.getByText("queued-two")).toBeVisible();
  await expect(watcher.getByText("queued-three")).toBeVisible();

  const delivered = await watcher.getByText(/^queued-/).count();
  expect(delivered).toBe(3);

  await senderContext.close();
  await watcherContext.close();
});

test("leaving releases the nick and clears the local cache", async ({ browser }) => {
  const first = await browser.newContext();
  const leaver = await first.newPage();

  await join(leaver, "leaver");
  await say(leaver, "said before leaving");
  await expect(leaver.getByText("said before leaving")).toBeVisible();

  await say(leaver, "/leave");
  await expect(leaver.getByLabel("nick")).toBeVisible();

  const cached = await leaver.evaluate(async () => {
    const databases = await indexedDB.databases();
    return databases.map((entry) => entry.name);
  });
  expect(cached).toContain("termchat");

  const second = await browser.newContext();
  const reclaimer = await second.newPage();
  await join(reclaimer, "leaver");
  await expect(reclaimer.getByRole("log", { name: "transcript" })).toBeVisible();

  await first.close();
  await second.close();
});
