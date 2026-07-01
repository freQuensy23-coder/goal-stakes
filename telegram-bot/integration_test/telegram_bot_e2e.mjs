#!/usr/bin/env node
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { once } from "node:events";

const token = "123456:test-token";
const botSecret = "telegram-internal-secret";
const rawAPIKeyThatMustNotBeUsed = "sk_bad_telegram_e2e";
const telegramPort = await freePort();
const apiPort = await freePort();
const telegramBase = `http://127.0.0.1:${telegramPort}`;
const apiBase = `http://127.0.0.1:${apiPort}`;
const sentMessages = [];
const backendMessageCalls = [];
const backendAudioCalls = [];
const backendAgentCalls = [];

const updates = [
  messageUpdate(1, 42, "private", 101, "/start"),
  messageUpdate(2, 42, "private", 102, `/apikey ${rawAPIKeyThatMustNotBeUsed}`),
  messageUpdate(3, 42, "private", 103, "/link BADCODE"),
  messageUpdate(4, 42, "private", 104, "/link ABCD1234"),
  messageUpdate(5, 42, "private", 105, "/goals"),
  messageUpdate(6, 42, "private", 106, "/create do daily 1 USDC sepolia Telegram push-ups"),
  messageUpdate(7, 42, "private", 107, "/done goal-1 finished today"),
  messageUpdate(8, 42, "private", 108, "/violate goal-2 drank soda"),
  messageUpdate(9, 42, "private", 109, "/progress goal-1"),
  messageUpdate(10, 42, "private", 110, "/archive goal-1"),
  messageUpdate(11, 42, "private", 111, "I did 10 push-ups"),
  messageUpdate(12, -55, "group", 201, "/goals"),
  channelPostUpdate(13, -100123, 301, "/goals"),
  channelPostVoiceUpdate(14, -100123, 302, "voice-file-id"),
  messageUpdate(15, 42, "private", 112, "/agent"),
];

let telegramServer;
let apiServer;
let bot;

try {
  telegramServer = await startTelegramServer();
  apiServer = await startGoalStakesServer();
  bot = spawn("go", ["run", "./cmd/telegram-bot"], {
    cwd: new URL("..", import.meta.url),
    env: {
      ...process.env,
      TELEGRAM_BOT_TOKEN: token,
      TELEGRAM_BOT_SECRET: botSecret,
      TELEGRAM_API_BASE: telegramBase,
      GOALSTAKES_API_BASE: apiBase,
      TELEGRAM_POLL_TIMEOUT_SECONDS: "1",
    },
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  const logs = [];
  bot.stdout.on("data", (chunk) => logs.push(chunk.toString()));
  bot.stderr.on("data", (chunk) => logs.push(chunk.toString()));

  await waitForReplies(15, 20000);
  await stopBot(bot);

  assertReplies();
  const joinedLogs = logs.join("");
  if (joinedLogs.includes(token) || joinedLogs.includes(botSecret) || joinedLogs.includes(rawAPIKeyThatMustNotBeUsed)) {
    throw new Error(`telegram bot leaked token, bot secret, or API key in logs:\n${joinedLogs}`);
  }
  console.log("telegram bot e2e passed");
} finally {
  if (bot && bot.exitCode === null && bot.signalCode === null) {
    await stopBot(bot).catch(() => {});
  }
  if (telegramServer) await closeServer(telegramServer);
  if (apiServer) await closeServer(apiServer);
}

async function stopBot(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  try {
    process.kill(-child.pid, "SIGTERM");
  } catch {
    child.kill("SIGTERM");
  }
  const exited = once(child, "exit").then(() => true);
  const timedOut = sleep(3000).then(() => false);
  if (!(await Promise.race([exited, timedOut]))) {
    try {
      process.kill(-child.pid, "SIGKILL");
    } catch {
      child.kill("SIGKILL");
    }
    await once(child, "exit").catch(() => {});
  }
}

function startTelegramServer() {
  const server = createServer(async (req, res) => {
    try {
      const url = new URL(req.url ?? "/", telegramBase);
      if (url.pathname.endsWith("/getUpdates")) {
        const offset = Number(url.searchParams.get("offset") ?? "0");
        return json(res, 200, { ok: true, result: updates.filter((item) => item.update_id >= offset) });
      }
      if (url.pathname === `/bot${token}/getFile`) {
        const body = JSON.parse(await readBody(req));
        if (body.file_id !== "voice-file-id") throw new Error(`unexpected getFile body ${JSON.stringify(body)}`);
        return json(res, 200, { ok: true, result: { file_id: "voice-file-id", file_path: "voice/file.oga" } });
      }
      if (url.pathname === `/file/bot${token}/voice/file.oga`) {
        res.writeHead(200, { "Content-Type": "audio/ogg" });
        res.end("fake-ogg-voice");
        return;
      }
      if (url.pathname !== `/bot${token}/sendMessage`) {
        return json(res, 404, { ok: false, description: "not found" });
      }
      const body = JSON.parse(await readBody(req));
      sentMessages.push(body);
      return json(res, 200, { ok: true, result: { message_id: sentMessages.length } });
    } catch (error) {
      return json(res, 500, { ok: false, description: error instanceof Error ? error.message : String(error) });
    }
  });
  return listen(server, telegramPort);
}

function startGoalStakesServer() {
  const server = createServer(async (req, res) => {
    try {
      const url = new URL(req.url ?? "/", apiBase);
      if (req.headers.authorization !== `Bearer ${botSecret}`) {
        return json(res, 401, { error: "Unauthorized" });
      }
      if (req.method === "POST" && url.pathname === "/internal/telegram/link") {
        const body = JSON.parse(await readBody(req));
        if (body.chat_id !== 42 || body.chat_kind !== "private") throw new Error(`unexpected telegram link body ${JSON.stringify(body)}`);
        if (body.code !== "ABCD1234") return json(res, 400, { error: "service: invalid input: invalid or expired telegram link code" });
        return json(res, 200, { reply: "Linked to Goal Stakes.", telegram_link: { chat_id: 42, chat_kind: "private" } });
      }
      if (req.method === "POST" && url.pathname === "/internal/telegram/message") {
        const body = JSON.parse(await readBody(req));
        backendMessageCalls.push(body);
        if (body.chat_id === 42 && body.chat_kind === "private") {
          if (body.text === "/goals") return json(res, 200, { reply: "No active goals." });
          if (body.text.startsWith("/create ")) return json(res, 200, { reply: "Created goal Telegram push-ups: goal-1" });
          if (body.text.startsWith("/done ")) return json(res, 200, { reply: "Check-in recorded for 2026-05-25." });
          if (body.text.startsWith("/violate ")) return json(res, 200, { reply: "Violation recorded for 2026-05-25 with status pending." });
          if (body.text.startsWith("/progress ")) return json(res, 200, { reply: "Telegram push-ups: goal-1\nperiod: 2026-05-25, completed: yes\nviolations: 0" });
          if (body.text.startsWith("/archive ")) return json(res, 200, { reply: "Goal archived." });
          if (body.text === "I did 10 push-ups") return json(res, 200, { reply: "AI reply: check-in recorded." });
        }
        if (body.chat_kind === "group") return json(res, 200, { reply: "Group command forwarded." });
        if (body.chat_kind === "channel") return json(res, 200, { reply: "Channel post forwarded." });
        return json(res, 400, { error: `unexpected telegram message body ${JSON.stringify(body)}` });
      }
      if (req.method === "POST" && url.pathname === "/internal/telegram/audio") {
        const contentType = req.headers["content-type"] ?? "";
        if (!String(contentType).startsWith("multipart/form-data; boundary=")) throw new Error(`unexpected audio content-type ${contentType}`);
        const body = await readBody(req);
        backendAudioCalls.push(body);
        for (const expected of [
          `name="chat_id"`,
          "-100123",
          `name="chat_kind"`,
          "channel",
          `name="message_id"`,
          "302",
          `name="audio"; filename="voice-file-id.ogg"`,
          "Content-Type: audio/ogg",
          "fake-ogg-voice",
        ]) {
          if (!body.includes(expected)) throw new Error(`audio multipart missing ${JSON.stringify(expected)} in:\n${body}`);
        }
        return json(res, 200, { transcript: "я отжался 10 раз", conversation_id: "00000000-0000-0000-0000-000000000001", reply: "Записал: 10 отжиманий" });
      }
      if (req.method === "POST" && url.pathname === "/internal/telegram/agent-link") {
        const body = JSON.parse(await readBody(req));
        backendAgentCalls.push(body);
        if (body.chat_id !== 42 || body.chat_kind !== "private" || body.name !== "telegram") throw new Error(`unexpected telegram agent-link body ${JSON.stringify(body)}`);
        return json(res, 201, { reply: "Own-agent skill link: https://api.goalstakes.test/agent-skills/agt_private.md", skill_url: "https://api.goalstakes.test/agent-skills/agt_private.md" });
      }
      return json(res, 404, { error: "not found" });
    } catch (error) {
      return json(res, 500, { error: error instanceof Error ? error.message : String(error) });
    }
  });
  return listen(server, apiPort);
}

function assertReplies() {
  const text = sentMessages.map((item) => String(item.text ?? "")).join("\n");
  for (const expected of [
    "Goal Stakes Telegram commands",
    "Use /link",
    "Could not link Telegram chat",
    "Linked to Goal Stakes.",
    "No active goals.",
    "Created goal Telegram push-ups",
    "Check-in recorded",
    "Violation recorded",
    "completed: yes",
    "Goal archived.",
    "AI reply: check-in recorded.",
    "Group command forwarded.",
    "Channel post forwarded.",
    "Записал: 10 отжиманий",
    "Own-agent skill link: https://api.goalstakes.test/agent-skills/agt_private.md",
  ]) {
    if (!text.includes(expected)) {
      throw new Error(`missing Telegram reply ${JSON.stringify(expected)} in:\n${text}`);
    }
  }
  if (text.includes(rawAPIKeyThatMustNotBeUsed) || text.includes(botSecret)) {
    throw new Error("Telegram replies leaked raw API key or bot secret");
  }
  if (backendAudioCalls.length !== 1) {
    throw new Error(`expected 1 backend audio call, got ${backendAudioCalls.length}`);
  }
  if (backendAgentCalls.length !== 1) {
    throw new Error(`expected 1 backend agent-link call, got ${backendAgentCalls.length}`);
  }
  const bodies = backendMessageCalls.map((item) => `${item.chat_id}:${item.chat_kind}:${item.message_id}:${item.text}`);
  for (const expected of [
    "42:private:105:/goals",
    "42:private:106:/create do daily 1 USDC sepolia Telegram push-ups",
    "42:private:111:I did 10 push-ups",
    "-55:group:201:/goals",
    "-100123:channel:301:/goals",
  ]) {
    if (!bodies.includes(expected)) {
      throw new Error(`missing backend message call ${expected}; got ${JSON.stringify(bodies)}`);
    }
  }
}

async function waitForReplies(count, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (sentMessages.length >= count) return;
    await sleep(100);
  }
  throw new Error(`timed out waiting for ${count} replies; got ${sentMessages.length}: ${JSON.stringify(sentMessages)}`);
}

function messageUpdate(id, chatID, chatType, messageID, text) {
  return { update_id: id, message: { message_id: messageID, chat: { id: chatID, type: chatType }, text } };
}

function channelPostUpdate(id, chatID, messageID, text) {
  return { update_id: id, channel_post: { message_id: messageID, chat: { id: chatID, type: "channel" }, text } };
}

function channelPostVoiceUpdate(id, chatID, messageID, fileID) {
  return { update_id: id, channel_post: { message_id: messageID, chat: { id: chatID, type: "channel" }, voice: { file_id: fileID, duration: 2, mime_type: "audio/ogg" } } };
}

function json(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  if (status !== 204) res.end(JSON.stringify(body));
  else res.end();
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => resolve(body));
    req.on("error", reject);
  });
}

function listen(server, port) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", () => resolve(server));
  });
}

function closeServer(server) {
  return new Promise((resolve) => server.close(resolve));
}

function freePort() {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      const port = typeof address === "object" && address ? address.port : 0;
      server.close(() => resolve(port));
    });
  });
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
