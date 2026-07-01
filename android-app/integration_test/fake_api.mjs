#!/usr/bin/env node
import { createServer } from "node:http";

const port = Number(process.env.GOALSTAKES_ANDROID_API_PORT || "18080");
const seedGoal = {
  id: "android-fake-goal",
  title: "Android smoke goal",
  description: "Loaded from the emulator smoke API",
  type: "do",
  cadence: "daily",
  stake_amount: "1000000",
  token_symbol: "USDC",
  chain: "sepolia",
  starts_at: "2026-06-29T00:00:00Z",
  ends_at: "",
};
let goals = [{ ...seedGoal }];

const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
  if (req.method === "GET" && url.pathname === "/api/v1/goals") {
    return json(res, 200, goals);
  }
  const goalID = matchGoalPath(url.pathname);
  if (req.method === "GET" && goalID && url.pathname.endsWith("/progress")) {
    const goal = findGoal(goalID);
    if (!goal) return json(res, 404, { error: "goal not found" });
    return json(res, 200, {
      goal,
      current_period: "2026-06-29",
      current_period_completed: true,
      violations: [],
    });
  }
  if (req.method === "POST" && url.pathname === "/api/v1/goals") {
    const body = JSON.parse(await readBody(req));
    const created = {
      ...seedGoal,
      id: `android-created-${goals.length + 1}`,
      title: body.title || "Created from Android",
      description: body.description || "",
      type: body.type || seedGoal.type,
      cadence: body.cadence || seedGoal.cadence,
      stake_amount: body.stake_amount || seedGoal.stake_amount,
      token_symbol: body.token_symbol || seedGoal.token_symbol,
      chain: body.chain || seedGoal.chain,
      starts_at: body.starts_at || "",
      ends_at: body.ends_at || "",
    };
    goals.push(created);
    return json(res, 201, created);
  }
  if (req.method === "PATCH" && goalID && url.pathname.endsWith("/stake")) {
    const goal = findGoal(goalID);
    if (!goal) return json(res, 404, { error: "goal not found" });
    const body = JSON.parse(await readBody(req));
    Object.assign(goal, {
      stake_amount: body.stake_amount || goal.stake_amount,
      token_symbol: body.token_symbol || goal.token_symbol,
      chain: body.chain || goal.chain,
    });
    return json(res, 200, goal);
  }
  if (req.method === "PATCH" && goalID) {
    const goal = findGoal(goalID);
    if (!goal) return json(res, 404, { error: "goal not found" });
    const body = JSON.parse(await readBody(req));
    Object.assign(goal, {
      title: body.title || goal.title,
      description: body.description ?? goal.description,
      stake_amount: body.stake_amount || goal.stake_amount,
      ends_at: body.ends_at === null ? "" : (body.ends_at ?? goal.ends_at),
    });
    return json(res, 200, goal);
  }
  if (req.method === "POST" && url.pathname.endsWith("/checkins")) {
    return json(res, 201, { id: "checkin-1" });
  }
  if (req.method === "POST" && url.pathname.endsWith("/violations")) {
    return json(res, 201, { id: "violation-1", status: "failed" });
  }
  if (req.method === "DELETE" && goalID) {
    goals = goals.filter((goal) => goal.id !== goalID);
    res.writeHead(204);
    return res.end();
  }
  if (req.method === "POST" && url.pathname === "/api/v1/chat") {
    return json(res, 200, { conversation_id: "00000000-0000-0000-0000-000000000001", reply: "Android smoke chat reply." });
  }
  if (req.method === "POST" && url.pathname === "/api/v1/agent-links") {
    return json(res, 201, {
      skill_url: `http://10.0.2.2:${port}/agent-skills/agt_android.md`,
      agent_link: {
        id: "android-agent-link",
        api_key_id: "android-agent-key",
        name: "android",
        expires_at: "2026-09-29T00:00:00Z",
        created_at: "2026-07-01T00:00:00Z",
      },
    });
  }
  return json(res, 404, { error: "not found" });
});

server.listen(port, "127.0.0.1", () => {
  console.log(`android fake api listening on ${port}`);
});

process.on("SIGTERM", () => server.close(() => process.exit(0)));

function json(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

function matchGoalPath(pathname) {
  const match = pathname.match(/^\/api\/v1\/goals\/([^/]+)/);
  return match?.[1] ?? "";
}

function findGoal(goalID) {
  return goals.find((goal) => goal.id === goalID);
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
