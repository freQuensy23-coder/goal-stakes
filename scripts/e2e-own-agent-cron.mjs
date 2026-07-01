#!/usr/bin/env node
import { createServer } from "node:http";

const agentSecret = "sk_agent_cron_private_value";
const skillToken = "agt_cron_private";
let goalsMode = "active";
let revoked = false;

const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://${req.headers.host}`);
  if (req.method === "GET" && url.pathname === `/agent-skills/${skillToken}.md`) {
    if (revoked) return json(res, 404, { error: "not found" });
    res.writeHead(200, { "Content-Type": "text/markdown; charset=utf-8" });
    res.end(skillMarkdown(`http://${req.headers.host}`, agentSecret));
    return;
  }
  if (req.method === "GET" && url.pathname === "/api/v1/goals") {
    if (revoked || req.headers.authorization !== `Bearer ${agentSecret}`) {
      return json(res, 401, { error: "Unauthorized" });
    }
    return json(res, 200, goalsMode === "active" ? [
      { id: "goal-1", title: "Morning push-ups", archived: false, cadence: "daily" },
      { id: "goal-archived", title: "Archived goal", archived: true, cadence: "daily" },
    ] : []);
  }
  json(res, 404, { error: "not found" });
});

try {
  await listen(server);
  const baseURL = `http://127.0.0.1:${server.address().port}`;
  const skill = await requestText(`${baseURL}/agent-skills/${skillToken}.md`);
  for (const expected of [
    "Run once per day in the user's timezone.",
    "GET /api/v1/goals",
    "If at least one active unarchived goal exists",
    "Do not mark a goal done from the reminder alone",
    "Authorization: Bearer sk_",
  ]) {
    if (!skill.includes(expected)) throw new Error(`skill missing ${expected}`);
  }
  const extractedSecret = skill.match(/Authorization: Bearer (sk_[^\s]+)/)?.[1] ?? "";
  if (extractedSecret !== agentSecret) throw new Error(`secret extraction failed: ${extractedSecret}`);

  const messages = [];
  await runDailyReminder(baseURL, extractedSecret, (message) => messages.push(message));
  if (messages.length !== 1 || !messages[0].includes("Morning push-ups") || !messages[0].includes("check in or report a violation")) {
    throw new Error(`active goals did not produce the expected reminder: ${JSON.stringify(messages)}`);
  }
  if (messages[0].includes("Archived goal") || messages[0].includes("sk_")) {
    throw new Error(`reminder leaked archived goal or secret: ${messages[0]}`);
  }

  goalsMode = "empty";
  await runDailyReminder(baseURL, extractedSecret, (message) => messages.push(message));
  if (messages.length !== 1) throw new Error(`empty goals should not send a reminder: ${JSON.stringify(messages)}`);

  revoked = true;
  await expectRejects401(() => runDailyReminder(baseURL, extractedSecret, () => {}));
  console.log("own-agent cron e2e passed");
} finally {
  await closeServer(server);
}

async function runDailyReminder(apiBase, secret, sendReminder) {
  const response = await fetch(`${apiBase}/api/v1/goals`, {
    headers: { Authorization: `Bearer ${secret}` },
  });
  const text = await response.text();
  if (!response.ok) throw new Error(`GET /api/v1/goals returned ${response.status}: ${text}`);
  const goals = JSON.parse(text);
  const active = goals.filter((goal) => !goal.archived);
  if (active.length === 0) return;
  sendReminder(`Daily Goal Stakes reminder: check in or report a violation for ${active.map((goal) => goal.title).join(", ")}.`);
}

function skillMarkdown(apiBase, secret) {
  return `---
name: goal-stakes-user-agent
description: Use when this user asks to manage Goal Stakes goals, check progress, send reminders, or record goal updates through the Goal Stakes API.
---

# Goal Stakes User Agent Skill

API base: ${apiBase}
Authorization: Bearer ${secret}

Daily cron:
- Run once per day in the user's timezone.
- Call GET /api/v1/goals with the Authorization header above.
- If at least one active unarchived goal exists, remind the user to check in or report a violation.
- If no active goals exist, send no reminder.
- Do not mark a goal done from the reminder alone; wait for explicit user confirmation.
`;
}

async function requestText(url) {
  const response = await fetch(url);
  const text = await response.text();
  if (!response.ok) throw new Error(`${url} returned ${response.status}: ${text}`);
  return text;
}

async function expectRejects401(fn) {
  try {
    await fn();
  } catch (error) {
    if (String(error).includes("401")) return;
    throw error;
  }
  throw new Error("revoked generated key unexpectedly worked");
}

function json(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

function listen(server) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
}

function closeServer(server) {
  return new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });
}
