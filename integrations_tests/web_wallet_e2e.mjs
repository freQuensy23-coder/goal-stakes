#!/usr/bin/env node
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { createRequire } from "node:module";
import { once } from "node:events";
import { existsSync } from "node:fs";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");
const backendDir = path.join(root, "backend");
const frontendDir = path.join(root, "frontend");
const manualWebEvidenceDir = path.join(root, ".e2e", "manual-web");
const requireFromFrontend = createRequire(path.join(frontendDir, "package.json"));
const { generatePrivateKey, privateKeyToAccount } = await import(requireFromFrontend.resolve("viem/accounts"));

const account = privateKeyToAccount(generatePrivateKey());
const apiPort = await freePort();
const webPort = await freePort();
const signerPort = await freePort();
const llmPort = await freePort();
const chromePort = await freePort();
const apiBase = `http://127.0.0.1:${apiPort}`;
const webBase = `http://127.0.0.1:${webPort}`;
const signerBase = `http://127.0.0.1:${signerPort}`;
const llmBase = `http://127.0.0.1:${llmPort}`;
const ethereumEnforcer = "0x1111111111111111111111111111111111111111";
const polygonEnforcer = "0x2222222222222222222222222222222222222222";
const ethereumUSDC = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48";
const ethereumUSDT = "0xdAC17F958D2ee523a2206206994597C13D831ec7";
const polygonUSDC = "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359";
const polygonUSDT = "0xc2132D05D31c914a87C6611C10748AEb04B58e8F";
const goalTitle = `E2E push-ups ${Date.now()}`;
const aiGoalTitle = `Do 100 push-ups every day ${Date.now()}`;
const children = [];
let signerServer;
let llmServer;
let chromeProfile;
let page;

async function main() {
try {
  await rm(manualWebEvidenceDir, { recursive: true, force: true });
  await mkdir(manualWebEvidenceDir, { recursive: true });
  signerServer = await startSignerServer(signerPort, account);
  llmServer = await startFakeOpenAIServer(llmPort, aiGoalTitle, goalTitle);

  const api = startProcess("api", "go", ["run", "./cmd/api"], {
    cwd: backendDir,
    env: {
      ...process.env,
      HTTP_PORT: String(apiPort),
      DATABASE_URL: process.env.DATABASE_URL ?? "postgres://goalstakes:goalstakes@localhost:5433/goalstakes?sslmode=disable",
      JWT_SECRET: "goalstakes-e2e-jwt-secret",
      SIWE_DOMAIN: `127.0.0.1:${webPort}`,
      SESSION_TTL: "24h",
      SCHEDULER_INTERVAL: "1h",
      OPENAI_API_KEY: "sk-e2e",
      OPENAI_MODEL: "gpt-e2e",
      OPENAI_TRANSCRIPTION_MODEL: "whisper-e2e",
      OPENAI_BASE_URL: `${llmBase}/v1`,
      ENFORCER_PRIVATE_KEY: "",
      ALLOW_DISABLED_ENFORCER: "true",
      CHAINS_JSON: JSON.stringify({
        ethereum: {
          rpc_url: "https://mainnet.example/rpc",
          stake_enforcer_address: ethereumEnforcer,
          tokens: {
            USDC: ethereumUSDC,
            USDT: ethereumUSDT,
          },
        },
        polygon: {
          rpc_url: "https://polygon.example/rpc",
          stake_enforcer_address: polygonEnforcer,
          tokens: {
            USDC: polygonUSDC,
            USDT: polygonUSDT,
          },
        },
      }),
    },
  });
  children.push(api);
  await waitForHTTP(`${apiBase}/api/v1/chains`, "api", api);
  assertChainTokenMatrix(await requestJSON(`${apiBase}/api/v1/chains`));

  const vite = startProcess("vite", "npm", ["run", "dev", "--", "--port", String(webPort), "--strictPort"], {
    cwd: frontendDir,
    env: { ...process.env, VITE_API_BASE_URL: apiBase },
  });
  children.push(vite);
  await waitForHTTP(webBase, "vite", vite);

  chromeProfile = await mkdtemp(path.join(tmpdir(), "goalstakes-chrome-"));
  const chrome = startProcess("chrome", chromePath(), [
    "--headless=new",
    "--disable-gpu",
    "--no-first-run",
    "--no-default-browser-check",
    `--remote-debugging-port=${chromePort}`,
    `--user-data-dir=${chromeProfile}`,
    "about:blank",
  ]);
  children.push(chrome);
  page = await connectToFirstChromePage(chromePort, chrome);
  const browserErrors = [];
  page.on("Runtime.exceptionThrown", (event) => {
    browserErrors.push(event.exceptionDetails?.text ?? "browser exception");
  });
  page.on("Runtime.consoleAPICalled", (event) => {
    if (event.type === "error") {
      browserErrors.push(event.args?.map((arg) => arg.value ?? arg.description ?? "").join(" ") ?? "console error");
    }
  });
  await page.send("Page.enable");
  await page.send("Runtime.enable");
  await setViewport(page, 1440, 900);
  await page.send("Page.addScriptToEvaluateOnNewDocument", {
    source: ethereumMockScript(account.address, signerBase),
  });
  await page.send("Page.addScriptToEvaluateOnNewDocument", {
    source: staleWalletStorageScript(webBase),
  });
  await page.send("Page.addScriptToEvaluateOnNewDocument", {
    source: voiceRecognitionMockScript("Create a weekly gym goal"),
  });
  await page.send("Page.navigate", { url: webBase });
  await waitForExpression(page, "document.readyState === 'complete' || document.readyState === 'interactive'", "page load");

  await waitForText(page, "Goal Stakes");
  await assertLandingHasNoFakeCredentials(page);
  await assertAPIDocsLinks(page, apiBase);
  await assertNoHorizontalOverflow(page, "landing desktop");
  await captureScreenshot(page, "landing-desktop");
  await setViewport(page, 390, 844);
  await waitForText(page, "Goal Stakes");
  await assertNoHorizontalOverflow(page, "landing mobile");
  await captureScreenshot(page, "landing-mobile");
  await setViewport(page, 1440, 900);
  await evaluate(page, "window.__goalstakesWalletEvents.rejectNextSignature = true");
  await clickByText(page, "Connect MetaMask");
  await waitForText(page, "User rejected signing");
  await assertNoHorizontalOverflow(page, "wallet rejection desktop");
  await captureScreenshot(page, "wallet-signature-rejected-desktop");
  await clickByText(page, "Connect MetaMask");
  await waitForText(page, "Token Approval");
  await assertNoHorizontalOverflow(page, "approval gate desktop");
  await captureScreenshot(page, "approval-gate-desktop");
  const storedWalletAfterLogin = await evaluate(page, 'localStorage.getItem("goalstakes.wallet") ?? ""');
  if (storedWalletAfterLogin.toLowerCase() !== account.address.toLowerCase()) {
    throw new Error(`SIWE login used stale wallet storage: stored=${storedWalletAfterLogin} current=${account.address}`);
  }
  const sessionAfterLogin = await evaluate(page, 'localStorage.getItem("goalstakes.session") ?? ""');
  if (!sessionAfterLogin) throw new Error("SIWE login did not store a session token");
  await expectRequestError(`${apiBase}/api/v1/approvals`, {
    method: "POST",
    headers: { Authorization: `Bearer ${sessionAfterLogin}`, "Content-Type": "application/json" },
    body: JSON.stringify({ chain: "ethereum", token_symbol: "USDC", allowance: "100000000" }),
  }, "invalid json");
  await expectRequestError(`${apiBase}/api/v1/approvals`, {
    method: "POST",
    headers: { Authorization: `Bearer ${sessionAfterLogin}`, "Content-Type": "application/json" },
    body: JSON.stringify({ chain: "ethereum", token_symbol: "USDC", dry_run_allowance: "100000000" }),
  }, "tx_hash is required");
  const launchApprovalCount = await walletTransactionCount(page);
  await evaluate(page, "window.__goalstakesWalletEvents.revertNextReceipt = true");
  await clickByText(page, "Approve and continue");
  await waitForText(page, "Approval transaction reverted");
  await assertApprovalTx(page, {
    chainID: "0x1",
    token: ethereumUSDC,
    spender: ethereumEnforcer,
    amount: "100000000",
    afterTransactionCount: launchApprovalCount,
  });
  const approvalAfterRejectedTx = await requestJSON(`${apiBase}/api/v1/approvals?chain=ethereum&token_symbol=USDC`, {
    headers: { Authorization: `Bearer ${sessionAfterLogin}` },
  });
  if (approvalAfterRejectedTx.allowance !== "0") {
    throw new Error(`reverted approval transaction was recorded as allowance ${approvalAfterRejectedTx.allowance}`);
  }
  await captureScreenshot(page, "approval-reverted-desktop");
  const successfulApprovalCount = await walletTransactionCount(page);
  await clickByText(page, "Approve and continue");
  await waitForText(page, "Chat");
  await assertNoHorizontalOverflow(page, "chat desktop");
  await captureScreenshot(page, "chat-desktop");
  await assertApprovalTx(page, {
    chainID: "0x1",
    token: ethereumUSDC,
    spender: ethereumEnforcer,
    amount: "100000000",
    afterTransactionCount: successfulApprovalCount,
  });

  await assertVoiceInputSupportedPath(page);
  await assertVoiceInputUnsupportedPath(page);
  await captureScreenshot(page, "chat-voice-desktop");

  await setInputValue(page, '.chat-composer input[placeholder="Message"]', "Create a daily push-up goal with a $100 stake");
  await clickByText(page, "Send");
  await waitForText(page, "Created the push-up goal.");
  await setInputValue(page, '.chat-composer input[placeholder="Message"]', "I did 10 push-ups.");
  await clickByText(page, "Send");
  await waitForText(page, "Recorded 10 push-ups.");
  const audioForm = new FormData();
  audioForm.append("audio", new Blob(["fake-ogg-audio"], { type: "audio/ogg" }), "voice.ogg");
  const audioChat = await requestJSON(`${apiBase}/api/v1/chat/audio`, {
    method: "POST",
    headers: { Authorization: `Bearer ${sessionAfterLogin}` },
    body: audioForm,
  });
  if (audioChat.transcript !== "I did 10 push-ups." || audioChat.reply !== "Recorded 10 push-ups." || !audioChat.conversation_id) {
    throw new Error(`audio chat response did not include transcript/reply/conversation id: ${JSON.stringify(audioChat)}`);
  }

  await clickByText(page, "Goals");
  await waitForText(page, aiGoalTitle);
  await setInputValue(page, 'input[placeholder="Do 100 push-ups every day"]', goalTitle);
  await setSelectValue(page, ".goal-form select", "avoid");
  await setInputValue(page, '.goal-form input[aria-label="Start date"]', "2026-05-25");
  await setInputValue(page, '.goal-form input[aria-label="End date"]', "2026-06-05");
  await setInputValue(page, '.goal-form input[aria-label="Stake amount"]', "1.0000001");
  await clickByText(page, "Create");
  await waitForText(page, "USDC/USDT stakes support up to 6 decimals");
  await setInputValue(page, '.goal-form input[aria-label="Stake amount"]', "100");
  await clickByText(page, "Create");
  await waitForText(page, goalTitle);
  await clickGoalAction(page, goalTitle, "Violation");
  await waitForGoalText(page, goalTitle, "1 violation");
  await clickGoalAction(page, goalTitle, "Violation");
  await waitForGoalText(page, goalTitle, "2 violations");
  await assertNoHorizontalOverflow(page, "goals desktop");
  await captureScreenshot(page, "goals-desktop");

  const token = await evaluate(page, 'localStorage.getItem("goalstakes.session")');
  if (!token) throw new Error("browser did not persist a session token");
  await page.send("Page.reload", { ignoreCache: true });
  await waitForText(page, "Chat");
  const tokenAfterReload = await evaluate(page, 'localStorage.getItem("goalstakes.session")');
  if (tokenAfterReload !== token) {
    throw new Error("reload did not preserve the active session token");
  }
  await clickByText(page, "Goals");
  await waitForText(page, goalTitle);
  const goals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${token}` } });
  const created = goals.find((goal) => goal.title === goalTitle);
  if (!created) throw new Error(`created goal ${goalTitle} was not returned by the API`);
  const aiCreated = goals.find((goal) => goal.title === aiGoalTitle);
  if (!aiCreated) throw new Error(`AI-created goal ${aiGoalTitle} was not returned by the API`);
  const voiceCreated = goals.find((goal) => String(goal.title ?? "").includes("Go to the gym every week"));
  if (!voiceCreated || voiceCreated.type !== "do" || voiceCreated.cadence !== "weekly") {
    throw new Error(`voice-created gym goal was not returned by the API with weekly do fields: ${JSON.stringify(voiceCreated)}`);
  }
  if (aiCreated.type !== "do" || aiCreated.cadence !== "daily" || aiCreated.stake_amount !== "100000000") {
    throw new Error(`AI-created goal had unexpected fields: ${JSON.stringify(aiCreated)}`);
  }
  const aiProgress = await requestJSON(`${apiBase}/api/v1/goals/${aiCreated.id}/progress`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!aiProgress.current_period_completed || !String(aiProgress.current_period_check_in?.note ?? "").includes("10 push-ups")) {
    throw new Error(`AI check-in did not record the push-up progress: ${JSON.stringify(aiProgress)}`);
  }
  if (created.type !== "avoid" || created.stake_amount !== "100000000" || created.token_symbol !== "USDC" || created.chain !== "ethereum") {
    throw new Error(`created goal had unexpected stake fields: ${JSON.stringify(created)}`);
  }
  if (localDateInput(created.starts_at) !== "2026-05-25" || localDateInput(created.ends_at) !== "2026-06-05") {
    throw new Error(`manual UI did not create the expected schedule window: ${JSON.stringify(created)}`);
  }
  await clickGoalButtonByTitle(page, goalTitle, "Edit goal");
  await setInputValue(page, '.goal-edit-form input[aria-label="End date"]', "2026-06-10");
  await clickWithin(page, ".goal-edit-form", "Save");
  await waitForGoalText(page, goalTitle, "ends");
  const manuallyEditedGoals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${token}` } });
  const manuallyEdited = manuallyEditedGoals.find((goal) => goal.id === created.id);
  if (!manuallyEdited || localDateInput(manuallyEdited.ends_at) !== "2026-06-10") {
    throw new Error(`manual UI did not update the goal end date: ${JSON.stringify(manuallyEdited)}`);
  }
  const progress = await requestJSON(`${apiBase}/api/v1/goals/${created.id}/progress`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (progress.violations.length !== 2) {
    throw new Error(`avoid goal violations len = ${progress.violations.length}, want 2`);
  }
  const approval = await requestJSON(`${apiBase}/api/v1/approvals?chain=ethereum&token_symbol=USDC`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (approval.allowance !== "100000000") {
    throw new Error(`recorded approval allowance = ${approval.allowance}, want 100000000`);
  }

  await clickByText(page, "Settings");
  await waitForText(page, "API Keys");
  await assertNoHorizontalOverflow(page, "settings desktop");
  await captureScreenshot(page, "settings-desktop");
  await setSelectValueAt(page, ".approval-panel", 0, "polygon");
  await setSelectValueAt(page, ".approval-panel", 1, "USDT");
  const polygonUSDTApprovalCount = await walletTransactionCount(page);
  await clickWithin(page, ".approval-panel", "Approve");
  await assertApprovalTx(page, {
    chainID: "0x89",
    token: polygonUSDT,
    spender: polygonEnforcer,
    amount: "100000000",
    afterTransactionCount: polygonUSDTApprovalCount,
  });
  await approveAndAssertMatrixCombo(page, token, {
    chain: "ethereum",
    tokenSymbol: "USDT",
    chainID: "0x1",
    tokenAddress: ethereumUSDT,
    spender: ethereumEnforcer,
  });
  await approveAndAssertMatrixCombo(page, token, {
    chain: "polygon",
    tokenSymbol: "USDC",
    chainID: "0x89",
    tokenAddress: polygonUSDC,
    spender: polygonEnforcer,
  });
  await assertRecordedApprovalMatrix(token);

  await clickByText(page, "Chat");
  await setInputValue(page, '.chat-composer input[placeholder="Message"]', "Move my soda goal stake to $70 USDT on Polygon.");
  await clickByText(page, "Send");
  await waitForText(page, "Moved the soda stake to Polygon USDT.");
  const aiRestakedGoals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${token}` } });
  const aiRestaked = aiRestakedGoals.find((goal) => goal.id === created.id);
  if (!aiRestaked || aiRestaked.stake_amount !== "70000000" || aiRestaked.token_symbol !== "USDT" || aiRestaked.chain !== "polygon") {
    throw new Error(`AI set_stake did not move the avoid goal to Polygon USDT: ${JSON.stringify(aiRestaked)}`);
  }
  await setInputValue(page, '.chat-composer input[placeholder="Message"]', "End my soda goal on June 15, 2026.");
  await clickByText(page, "Send");
  await waitForText(page, "Set the soda goal end date.");
  const aiEndedGoals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${token}` } });
  const aiEnded = aiEndedGoals.find((goal) => goal.id === created.id);
  if (!aiEnded || Date.parse(aiEnded.ends_at) !== Date.parse("2026-06-15T00:00:00Z")) {
    throw new Error(`AI update_goal did not set the avoid goal end date: ${JSON.stringify(aiEnded)}`);
  }
  await setInputValue(page, '.chat-composer input[placeholder="Message"]', "Remove the end date from my soda goal.");
  await clickByText(page, "Send");
  await waitForText(page, "Cleared the soda goal end date.");
  const aiClearedGoals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${token}` } });
  const aiCleared = aiClearedGoals.find((goal) => goal.id === created.id);
  if (!aiCleared || aiCleared.ends_at) {
    throw new Error(`AI update_goal did not clear the avoid goal end date: ${JSON.stringify(aiCleared)}`);
  }
  await setInputValue(page, '.chat-composer input[placeholder="Message"]', "I drank soda.");
  await clickByText(page, "Send");
  await waitForText(page, "Recorded the soda violation, but the local charger is disabled.");
  const afterAIViolation = await requestJSON(`${apiBase}/api/v1/goals/${created.id}/progress`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (afterAIViolation.violations.length !== 3) {
    throw new Error(`AI report_violation count = ${afterAIViolation.violations.length}, want 3`);
  }
  const aiViolation = afterAIViolation.violations[afterAIViolation.violations.length - 1];
  if (aiViolation.status !== "failed" || !String(aiViolation.reason ?? "").includes("drank soda through AI")) {
    throw new Error(`AI report_violation did not preserve the failed violation row: ${JSON.stringify(aiViolation)}`);
  }

  await clickByText(page, "Settings");
  await waitForText(page, "Do not paste raw");
  await clickByText(page, "Create link code");
  await waitForExpression(page, `document.querySelector('.telegram-link-code code')?.textContent?.length >= 8`, "Telegram link code");
  const telegramLinkCode = await evaluate(page, `document.querySelector('.telegram-link-code code')?.textContent ?? ''`);
  if (telegramLinkCode.startsWith("sk_")) {
    throw new Error(`Telegram link code must not be a raw API key: ${telegramLinkCode}`);
  }
  await clickByText(page, "Connect own agent");
  await waitForExpression(page, `document.querySelector('.agent-skill-link code')?.textContent?.includes('/agent-skills/agt_')`, "own-agent skill link");
  const agentSkillURL = await evaluate(page, `document.querySelector('.agent-skill-link code')?.textContent ?? ''`);
  if (!agentSkillURL.startsWith(`${apiBase}/agent-skills/agt_`) || !agentSkillURL.endsWith(".md")) {
    throw new Error(`own-agent skill URL has unexpected shape: ${agentSkillURL}`);
  }
  if (agentSkillURL.includes("sk_")) {
    throw new Error(`own-agent skill URL leaked raw API secret: ${agentSkillURL}`);
  }
  const agentSkillMarkdown = await requestText(agentSkillURL);
  for (const expected of ["Goal Stakes User Agent Skill", "Authorization: Bearer sk_", "GET /api/v1/goals", "Run once per day", "Never ask for wallet seed phrases"]) {
    if (!agentSkillMarkdown.includes(expected)) {
      throw new Error(`own-agent skill markdown missing ${expected}: ${agentSkillMarkdown}`);
    }
  }
  const agentSecret = agentSkillMarkdown.match(/Authorization: Bearer (sk_[^\s]+)/)?.[1] ?? "";
  if (!agentSecret) throw new Error(`could not extract generated agent secret from skill markdown: ${agentSkillMarkdown}`);
  const agentGoals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${agentSecret}` } });
  if (!agentGoals.some((goal) => goal.id === created.id)) {
    throw new Error("generated own-agent secret could not read active goals");
  }
  const agentLinks = await requestJSON(`${apiBase}/api/v1/agent-links`, { headers: { Authorization: `Bearer ${token}` } });
  const latestAgentLink = agentLinks.find((link) => link.name === "codex") ?? agentLinks[0];
  if (!latestAgentLink || JSON.stringify(agentLinks).includes("agt_") || JSON.stringify(agentLinks).includes("sk_")) {
    throw new Error(`agent link metadata leaked secret material or was missing: ${JSON.stringify(agentLinks)}`);
  }
  await requestNoContent(`${apiBase}/api/v1/agent-links/${latestAgentLink.id}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  await expectRequestError(`${apiBase}/api/v1/goals`, {
    headers: { Authorization: `Bearer ${agentSecret}` },
  }, "Unauthorized");
  await setInputValue(page, ".api-key-form input", `e2e-${Date.now()}`);
  await clickWithin(page, ".api-key-form", "Create");
  await waitForExpression(page, `document.querySelector('.api-key-created code')?.textContent?.startsWith('sk_')`, "raw API key");
  const apiKey = await evaluate(page, `document.querySelector('.api-key-created code')?.textContent ?? ''`);
  await captureScreenshot(page, "settings-api-key-created");
  const apiGoals = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${apiKey}` } });
  if (!apiGoals.some((goal) => goal.id === aiCreated.id) || !apiGoals.some((goal) => goal.id === created.id)) {
    throw new Error("public API key did not list goals created through chat and UI");
  }
  await requestJSON(`${apiBase}/api/v1/goals/${aiCreated.id}/checkins`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({ note: "checked in through public API key" }),
  });
  const apiProgress = await requestJSON(`${apiBase}/api/v1/goals/${aiCreated.id}/progress`, {
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  if (!apiProgress.current_period_completed) {
    throw new Error("public API key check-in did not mark the AI-created goal complete");
  }
  const apiLifecycleGoal = await requestJSON(`${apiBase}/api/v1/goals`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      title: `Public API lifecycle ${Date.now()}`,
      type: "avoid",
      cadence: "daily",
      stake_amount: "50000000",
      token_symbol: "USDC",
      chain: "ethereum",
      timezone: "UTC",
    }),
  });
  const updatedAPIGoal = await requestJSON(`${apiBase}/api/v1/goals/${apiLifecycleGoal.id}`, {
    method: "PATCH",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      title: `${apiLifecycleGoal.title} updated`,
      description: "updated through public API key",
    }),
  });
  if (updatedAPIGoal.title !== `${apiLifecycleGoal.title} updated` || updatedAPIGoal.description !== "updated through public API key" || updatedAPIGoal.stake_amount !== "50000000") {
    throw new Error(`public API key did not update goal fields: ${JSON.stringify(updatedAPIGoal)}`);
  }
  const restakedAPIGoal = await requestJSON(`${apiBase}/api/v1/goals/${apiLifecycleGoal.id}/stake`, {
    method: "PATCH",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      stake_amount: "70000000",
      token_symbol: "USDT",
      chain: "polygon",
    }),
  });
  if (restakedAPIGoal.stake_amount !== "70000000" || restakedAPIGoal.token_symbol !== "USDT" || restakedAPIGoal.chain !== "polygon") {
    throw new Error(`public API key did not update stake/token/chain: ${JSON.stringify(restakedAPIGoal)}`);
  }
  await expectRequestError(`${apiBase}/api/v1/goals/${apiLifecycleGoal.id}/violations`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({ reason: "full public API lifecycle violation" }),
  }, "ENFORCER_PRIVATE_KEY");
  const apiLifecycleViolations = await requestJSON(`${apiBase}/api/v1/goals/${apiLifecycleGoal.id}/violations`, {
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  if (apiLifecycleViolations.length !== 1 || apiLifecycleViolations[0].status !== "failed" || apiLifecycleViolations[0].amount !== "70000000") {
    throw new Error(`public API key did not list the expected failed violation: ${JSON.stringify(apiLifecycleViolations)}`);
  }
  await requestNoContent(`${apiBase}/api/v1/goals/${apiLifecycleGoal.id}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  const apiGoalsAfterArchive = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${apiKey}` } });
  if (apiGoalsAfterArchive.some((goal) => goal.id === apiLifecycleGoal.id)) {
    throw new Error("public API key archive did not remove the goal from active lists");
  }
  const beforeAPIViolation = await requestJSON(`${apiBase}/api/v1/goals/${created.id}/progress`, {
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  await expectRequestError(`${apiBase}/api/v1/goals/${created.id}/violations`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({ reason: "reported through public API key" }),
  }, "ENFORCER_PRIVATE_KEY");
  const afterAPIViolation = await requestJSON(`${apiBase}/api/v1/goals/${created.id}/progress`, {
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  if (afterAPIViolation.violations.length !== beforeAPIViolation.violations.length + 1) {
    throw new Error(`public API key violation count = ${afterAPIViolation.violations.length}, want ${beforeAPIViolation.violations.length + 1}`);
  }
  const apiViolation = afterAPIViolation.violations[afterAPIViolation.violations.length - 1];
  if (apiViolation.status !== "failed" || !String(apiViolation.reason ?? "").includes("public API key")) {
    throw new Error(`public API key violation was not recorded as the expected local charge failure: ${JSON.stringify(apiViolation)}`);
  }
  const apiCrossModuleTitle = `Public API cross-module ${Date.now()}`;
  const apiCrossModuleGoal = await requestJSON(`${apiBase}/api/v1/goals`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      title: apiCrossModuleTitle,
      type: "do",
      cadence: "daily",
      stake_amount: "1000000",
      token_symbol: "USDC",
      chain: "ethereum",
      timezone: "UTC",
    }),
  });
  if (apiCrossModuleGoal.title !== apiCrossModuleTitle || apiCrossModuleGoal.stake_amount !== "1000000") {
    throw new Error(`public API cross-module goal had unexpected fields: ${JSON.stringify(apiCrossModuleGoal)}`);
  }
  await setViewport(page, 1280, 900);
  await clickByText(page, "Goals");
  await waitForText(page, apiCrossModuleTitle);
  await assertNoHorizontalOverflow(page, "goals after public api create");
  await captureScreenshot(page, "goals-after-api-desktop");

  const androidVisibleTitle = await runAndroidApiIntegration(apiBase, apiKey, [goalTitle, apiCrossModuleTitle]);
  const goalsAfterAndroid = await requestJSON(`${apiBase}/api/v1/goals`, { headers: { Authorization: `Bearer ${apiKey}` } });
  if (!goalsAfterAndroid.some((goal) => goal.title === androidVisibleTitle)) {
    throw new Error(`Android-created goal was not visible through the shared backend: ${androidVisibleTitle}`);
  }
  await clickByText(page, "Goals");
  await clickByText(page, "Refresh");
  await waitForText(page, androidVisibleTitle);
  await assertNoHorizontalOverflow(page, "goals after android create");
  await captureScreenshot(page, "goals-after-android-desktop");

  await clickByText(page, "Settings");
  const apiKeys = await requestJSON(`${apiBase}/api/v1/apikeys`, { headers: { Authorization: `Bearer ${apiKey}` } });
  const activeKey = apiKeys.find((key) => key.prefix === apiKey.slice(0, 12));
  if (!activeKey) throw new Error(`created API key was not returned by /apikeys: ${JSON.stringify(apiKeys)}`);
  await requestNoContent(`${apiBase}/api/v1/apikeys/${activeKey.id}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  await expectRequestError(`${apiBase}/api/v1/goals`, {
    headers: { Authorization: `Bearer ${apiKey}` },
  }, "Unauthorized");
  await setViewport(page, 390, 844);
  await waitForText(page, "Settings");
  await assertNoHorizontalOverflow(page, "authenticated mobile settings");
  await captureScreenshot(page, "settings-mobile");
  await clickByTitle(page, "Sign out");
  await waitForText(page, "Connect MetaMask");
  const authAfterSignOut = await evaluate(page, `({
    session: localStorage.getItem("goalstakes.session"),
    wallet: localStorage.getItem("goalstakes.wallet"),
    conversation: localStorage.getItem("goalstakes.conversation"),
  })`);
  if (authAfterSignOut.session || authAfterSignOut.wallet || authAfterSignOut.conversation) {
    throw new Error(`sign out did not clear local auth state: ${JSON.stringify(authAfterSignOut)}`);
  }
  await assertNoHorizontalOverflow(page, "signed out mobile landing");
  if (browserErrors.length) {
    throw new Error(`browser reported errors:\n${browserErrors.join("\n")}`);
  }

  console.log("web wallet e2e passed");
} finally {
  if (page) page.close();
  for (const child of children.reverse()) {
    await stopProcess(child);
  }
  if (signerServer) await closeServer(signerServer);
  if (llmServer) await closeServer(llmServer);
  if (chromeProfile) await rm(chromeProfile, { recursive: true, force: true });
}
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

function startSignerServer(port, signerAccount) {
  const server = createServer(async (req, res) => {
    res.setHeader("Access-Control-Allow-Origin", "*");
    res.setHeader("Access-Control-Allow-Headers", "Content-Type");
    if (req.method === "OPTIONS") {
      res.writeHead(204);
      res.end();
      return;
    }
    if (req.method !== "POST" || req.url !== "/sign") {
      res.writeHead(404);
      res.end();
      return;
    }
    try {
      const body = JSON.parse(await readBody(req));
      if (body.address?.toLowerCase() !== signerAccount.address.toLowerCase()) {
        throw new Error("signer address mismatch");
      }
      const signature = await signerAccount.signMessage({ message: String(body.message ?? "") });
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ signature }));
    } catch (error) {
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: error instanceof Error ? error.message : "sign failed" }));
    }
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", () => resolve(server));
  });
}

function startFakeOpenAIServer(port, title, avoidTitle) {
  const server = createServer(async (req, res) => {
    if (req.method === "POST" && req.url === "/v1/audio/transcriptions") {
      const body = await readBody(req);
      if (!body.includes("whisper-e2e") || !body.includes("fake-ogg-audio")) {
        res.writeHead(400, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: { message: "audio transcription request missed model or file bytes" } }));
        return;
      }
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ text: "I did 10 push-ups." }));
      return;
    }
    if (req.method !== "POST" || req.url !== "/v1/chat/completions") {
      res.writeHead(404);
      res.end();
      return;
    }
    try {
      const body = JSON.parse(await readBody(req));
      const messages = Array.isArray(body.messages) ? body.messages : [];
      const latestUser = [...messages].reverse().find((message) => message.role === "user")?.content ?? "";
      const lastMessage = messages[messages.length - 1] ?? {};
      const lowerUser = latestUser.toLowerCase();

      if (lowerUser.includes("did 10 push-ups")) {
        if (lastMessage.role !== "tool") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-list-goals", "list_goals", {}), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-list-goals") {
          const goals = JSON.parse(lastMessage.content);
          const goal = goals.find((candidate) => String(candidate.title ?? "").includes(title));
          if (!goal) throw new Error(`list_goals result did not include ${title}`);
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-log-check-in", "log_check_in", {
            goal_id: goal.id,
            note: "10 push-ups",
          }), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-log-check-in") {
          if (!String(lastMessage.content ?? "").includes("10 push-ups")) {
            throw new Error("log_check_in result did not include the progress note");
          }
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse({ role: "assistant", content: "Recorded 10 push-ups." }, "stop")));
          return;
        }
        throw new Error(`unexpected tool result for progress message: ${lastMessage.tool_call_id}`);
      }

      if (lowerUser.includes("move my soda goal stake")) {
        if (lastMessage.role !== "tool") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-list-goals-for-stake", "list_goals", {}), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-list-goals-for-stake") {
          const goals = JSON.parse(lastMessage.content);
          const goal = goals.find((candidate) => String(candidate.title ?? "").includes(avoidTitle));
          if (!goal) throw new Error(`list_goals result did not include ${avoidTitle}`);
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-set-stake", "set_stake", {
            goal_id: goal.id,
            stake_amount: "70000000",
            token_symbol: "USDT",
            chain: "polygon",
          }), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-set-stake") {
          const result = JSON.parse(lastMessage.content);
          if (result.stake_amount !== "70000000" || result.token_symbol !== "USDT" || result.chain !== "polygon") {
            throw new Error(`set_stake result had unexpected fields: ${lastMessage.content}`);
          }
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse({ role: "assistant", content: "Moved the soda stake to Polygon USDT." }, "stop")));
          return;
        }
        throw new Error(`unexpected tool result for set_stake message: ${lastMessage.tool_call_id}`);
      }

      if (lowerUser.includes("end my soda goal")) {
        if (lastMessage.role !== "tool") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-list-goals-for-end-date", "list_goals", {}), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-list-goals-for-end-date") {
          const goals = JSON.parse(lastMessage.content);
          const goal = goals.find((candidate) => String(candidate.title ?? "").includes(avoidTitle));
          if (!goal) throw new Error(`list_goals result did not include ${avoidTitle}`);
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-update-end-date", "update_goal", {
            goal_id: goal.id,
            title: goal.title,
            description: goal.description || "",
            ends_at: "2026-06-15T00:00:00Z",
          }), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-update-end-date") {
          const result = JSON.parse(lastMessage.content);
          if (Date.parse(result.ends_at) !== Date.parse("2026-06-15T00:00:00Z")) {
            throw new Error(`update_goal result did not include the requested end date: ${lastMessage.content}`);
          }
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse({ role: "assistant", content: "Set the soda goal end date." }, "stop")));
          return;
        }
        throw new Error(`unexpected tool result for update_goal end date message: ${lastMessage.tool_call_id}`);
      }

      if (lowerUser.includes("remove the end date")) {
        if (lastMessage.role !== "tool") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-list-goals-for-clear-end-date", "list_goals", {}), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-list-goals-for-clear-end-date") {
          const goals = JSON.parse(lastMessage.content);
          const goal = goals.find((candidate) => String(candidate.title ?? "").includes(avoidTitle));
          if (!goal) throw new Error(`list_goals result did not include ${avoidTitle}`);
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-clear-end-date", "update_goal", {
            goal_id: goal.id,
            title: goal.title,
            description: goal.description || "",
            ends_at: null,
          }), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-clear-end-date") {
          const result = JSON.parse(lastMessage.content);
          if (result.ends_at) {
            throw new Error(`update_goal result should have cleared the end date: ${lastMessage.content}`);
          }
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse({ role: "assistant", content: "Cleared the soda goal end date." }, "stop")));
          return;
        }
        throw new Error(`unexpected tool result for clear end date message: ${lastMessage.tool_call_id}`);
      }

      if (lowerUser.includes("drank soda")) {
        if (lastMessage.role !== "tool") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-list-goals-for-violation", "list_goals", {}), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-list-goals-for-violation") {
          const goals = JSON.parse(lastMessage.content);
          const goal = goals.find((candidate) => String(candidate.title ?? "").includes(avoidTitle));
          if (!goal) throw new Error(`list_goals result did not include ${avoidTitle}`);
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse(toolCallMessage("call-report-violation", "report_violation", {
            goal_id: goal.id,
            reason: "drank soda through AI",
          }), "tool_calls")));
          return;
        }
        if (lastMessage.tool_call_id === "call-report-violation") {
          const result = JSON.parse(lastMessage.content);
          if (result.ok !== false || !String(result.error ?? "").includes("ENFORCER_PRIVATE_KEY")) {
            throw new Error(`report_violation result should surface the local charger failure: ${lastMessage.content}`);
          }
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(chatResponse({ role: "assistant", content: "Recorded the soda violation, but the local charger is disabled." }, "stop")));
          return;
        }
        throw new Error(`unexpected tool result for report_violation message: ${lastMessage.tool_call_id}`);
      }

      if (lastMessage.role !== "tool") {
        const isGym = lowerUser.includes("gym");
        const createTitle = isGym ? `Go to the gym every week ${Date.now()}` : title;
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(chatResponse(toolCallMessage("call-create-goal", "create_goal", {
          title: createTitle,
          type: "do",
          cadence: isGym ? "weekly" : "daily",
          stake_amount: "100000000",
          token_symbol: "USDC",
          chain: "ethereum",
        }), "tool_calls")));
        return;
      }
      if (lastMessage.tool_call_id !== "call-create-goal") {
        throw new Error(`unexpected tool result for create message: ${lastMessage.tool_call_id}`);
      }
      if (!String(lastMessage.content ?? "").includes("stake_amount")) {
        throw new Error("create_goal tool result did not include goal details");
      }
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(chatResponse({ role: "assistant", content: lowerUser.includes("gym") ? "Created the gym goal." : "Created the push-up goal." }, "stop")));
    } catch (error) {
      console.error("fake OpenAI server failed:", error instanceof Error ? error.message : error);
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: { message: error instanceof Error ? error.message : "fake LLM failed" } }));
    }
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", () => resolve(server));
  });
}

function toolCallMessage(id, name, args) {
  return {
    role: "assistant",
    tool_calls: [{
      id,
      type: "function",
      function: {
        name,
        arguments: JSON.stringify(args),
      },
    }],
  };
}

function chatResponse(message, finishReason) {
  return {
    id: "chatcmpl-goalstakes-e2e",
    object: "chat.completion",
    created: Math.floor(Date.now() / 1000),
    model: "gpt-e2e",
    choices: [{ index: 0, message, finish_reason: finishReason }],
  };
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

function startProcess(name, command, args, options = {}) {
  const child = spawn(command, args, {
    ...options,
    stdio: ["ignore", "pipe", "pipe"],
  });
  const logs = [];
  const push = (chunk) => {
    const text = chunk.toString();
    logs.push(text);
    if (logs.length > 80) logs.shift();
  };
  child.stdout.on("data", push);
  child.stderr.on("data", push);
  child.on("error", (error) => push(`${name} process error: ${error.message}\n`));
  return { name, child, logs };
}

async function runAndroidApiIntegration(baseURL, apiKey, expectedTitles = []) {
  const androidHome = process.env.ANDROID_HOME || path.join(process.env.HOME || "", "Library/Android/sdk");
  const title = `Android API e2e ${Date.now()}`;
  const visibleTitle = `${title} visible`;
  const expectedArg = expectedTitles.join("|");
  const proc = startProcess("android-api", "gradle", [
    `-Dgoalstakes.e2e.baseUrl=${baseURL}`,
    `-Dgoalstakes.e2e.apiKey=${apiKey}`,
    `-Dgoalstakes.e2e.goalTitle=${title}`,
    `-Dgoalstakes.e2e.expectedTitles=${expectedArg}`,
    "-Dgoalstakes.e2e.chain=ethereum",
    "testDebugUnitTest",
    "--tests",
    "com.goalstakes.mobile.ApiClientIntegrationTest",
  ], {
    cwd: path.join(root, "android-app"),
    env: { ...process.env, ANDROID_HOME: androidHome },
  });
  await waitForProcess(proc, 120000);
  const report = await readFile(path.join(root, "android-app/app/build/test-results/testDebugUnitTest/TEST-com.goalstakes.mobile.ApiClientIntegrationTest.xml"), "utf8");
  if (!report.includes('tests="1" skipped="0" failures="0" errors="0"')) {
    throw new Error(`Android API integration did not run cleanly:\n${report}`);
  }
  const stableDir = path.join(root, "android-app/build/e2e-results");
  await mkdir(stableDir, { recursive: true });
  await writeFile(path.join(stableDir, "android-api-integration.xml"), report);
  return visibleTitle;
}

async function waitForProcess(proc, timeoutMs) {
  const exited = once(proc.child, "exit").then(([code, signal]) => ({ code, signal, timedOut: false }));
  const timedOut = sleep(timeoutMs).then(() => ({ code: null, signal: null, timedOut: true }));
  const result = await Promise.race([exited, timedOut]);
  if (result.timedOut) {
    proc.child.kill("SIGKILL");
    throw new Error(`${proc.name} timed out:\n${proc.logs.join("")}`);
  }
  if (result.code !== 0) {
    throw new Error(`${proc.name} exited with code ${result.code ?? result.signal}:\n${proc.logs.join("")}`);
  }
}

async function stopProcess(proc) {
  if (proc.child.exitCode !== null || proc.child.signalCode !== null) return;
  proc.child.kill("SIGTERM");
  const exited = once(proc.child, "exit").then(() => true);
  const timedOut = sleep(3000).then(() => false);
  if (!(await Promise.race([exited, timedOut]))) {
    proc.child.kill("SIGKILL");
    await once(proc.child, "exit").catch(() => {});
  }
}

async function waitForHTTP(url, name, proc, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  let last = "";
  while (Date.now() < deadline) {
    if (proc.child.exitCode !== null) {
      throw new Error(`${name} exited before readiness:\n${proc.logs.join("")}`);
    }
    try {
      const response = await fetch(url);
      if (response.ok) return;
      last = `${response.status} ${response.statusText}`;
    } catch (error) {
      last = error instanceof Error ? error.message : String(error);
    }
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${name} at ${url}: ${last}\n${proc.logs.join("")}`);
}

async function requestJSON(url, options) {
  const response = await fetch(url, options);
  if (!response.ok) throw new Error(`${url} returned ${response.status}: ${await response.text()}`);
  return response.json();
}

async function requestText(url, options) {
  const response = await fetch(url, options);
  const text = await response.text();
  if (!response.ok) throw new Error(`${url} returned ${response.status}: ${text}`);
  return text;
}

async function requestNoContent(url, options) {
  const response = await fetch(url, options);
  const text = await response.text();
  if (response.status !== 204) {
    throw new Error(`${url} returned ${response.status}, want 204: ${text}`);
  }
}

async function expectRequestError(url, options, expectedText) {
  const response = await fetch(url, options);
  const text = await response.text();
  if (response.ok) {
    throw new Error(`${url} unexpectedly succeeded: ${text}`);
  }
  if (!text.includes(expectedText)) {
    throw new Error(`${url} returned ${response.status} without ${expectedText}: ${text}`);
  }
}

function chromePath() {
  const candidates = [
    process.env.CHROME_PATH,
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/usr/bin/google-chrome",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
  ].filter(Boolean);
  const found = candidates.find((candidate) => existsSync(candidate));
  if (!found) throw new Error("Chrome/Chromium was not found; set CHROME_PATH");
  return found;
}

async function connectToFirstChromePage(port, proc) {
  const deadline = Date.now() + 15000;
  let last = "";
  while (Date.now() < deadline) {
    if (proc.child.exitCode !== null) {
      throw new Error(`chrome exited before CDP readiness:\n${proc.logs.join("")}`);
    }
    try {
      const targets = await requestJSON(`http://127.0.0.1:${port}/json`);
      const target = targets.find((entry) => entry.type === "page" && entry.webSocketDebuggerUrl);
      if (target) return CDP.connect(target.webSocketDebuggerUrl);
    } catch (error) {
      last = error instanceof Error ? error.message : String(error);
    }
    await sleep(200);
  }
  throw new Error(`timed out waiting for Chrome CDP: ${last}\n${proc.logs.join("")}`);
}

class CDP {
  static connect(url) {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      ws.addEventListener("open", () => resolve(new CDP(ws)));
      ws.addEventListener("error", reject, { once: true });
    });
  }

  constructor(ws) {
    this.ws = ws;
    this.nextID = 1;
    this.pending = new Map();
    this.handlers = new Map();
    ws.addEventListener("message", (message) => {
      const payload = JSON.parse(message.data);
      if (payload.id && this.pending.has(payload.id)) {
        const { resolve, reject } = this.pending.get(payload.id);
        this.pending.delete(payload.id);
        if (payload.error) reject(new Error(payload.error.message));
        else resolve(payload.result ?? {});
        return;
      }
      const handlers = this.handlers.get(payload.method) ?? [];
      for (const handler of handlers) handler(payload.params ?? {});
    });
  }

  send(method, params = {}) {
    const id = this.nextID++;
    this.ws.send(JSON.stringify({ id, method, params }));
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
    });
  }

  on(method, handler) {
    const handlers = this.handlers.get(method) ?? [];
    handlers.push(handler);
    this.handlers.set(method, handlers);
  }

  close() {
    this.ws.close();
  }
}

async function evaluate(cdp, expression) {
  const result = await cdp.send("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) {
    const details = result.exceptionDetails;
    const description = details.exception?.description || details.exception?.value || details.text || "Runtime.evaluate failed";
    throw new Error(String(description));
  }
  return result.result?.value;
}

async function setViewport(cdp, width, height) {
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width,
    height,
    deviceScaleFactor: 1,
    mobile: width < 700,
  });
  await cdp.send("Emulation.setVisibleSize", { width, height }).catch(() => {});
}

async function captureScreenshot(cdp, name) {
  const result = await cdp.send("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
  });
  const file = path.join(manualWebEvidenceDir, `${name}.png`);
  await writeFile(file, Buffer.from(result.data, "base64"));
}

async function assertNoHorizontalOverflow(cdp, label) {
  const result = await evaluate(
    cdp,
    `(() => {
      const viewportWidth = window.innerWidth;
      const documentOverflow = Math.ceil(document.documentElement.scrollWidth - viewportWidth);
      const offenders = [...document.body.querySelectorAll('*')]
        .filter((el) => {
          const style = getComputedStyle(el);
          if (style.display === 'none' || style.visibility === 'hidden') return false;
          const rect = el.getBoundingClientRect();
          if (rect.width === 0 || rect.height === 0) return false;
          if (style.position === 'fixed' || style.position === 'absolute') return false;
          return rect.left < -1 || rect.right > viewportWidth + 1;
        })
        .slice(0, 8)
        .map((el) => ({
          tag: el.tagName.toLowerCase(),
          className: String(el.className || ''),
          text: (el.textContent || '').replace(/\\s+/g, ' ').trim().slice(0, 80),
          left: Math.round(el.getBoundingClientRect().left),
          right: Math.round(el.getBoundingClientRect().right),
          width: Math.round(el.getBoundingClientRect().width),
        }));
      return { viewportWidth, documentOverflow, offenders };
    })()`,
  );
  if (result.documentOverflow > 1 || result.offenders.length) {
    throw new Error(`${label} has horizontal overflow: ${JSON.stringify(result)}`);
  }
}

async function waitForExpression(cdp, expression, label, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await evaluate(cdp, `Boolean(${expression})`)) return;
    await sleep(100);
  }
  const body = await evaluate(cdp, "document.body?.innerText ?? ''").catch(() => "");
  throw new Error(`timed out waiting for ${label}\n${body}`);
}

async function waitForText(cdp, text, timeoutMs = 15000) {
  await waitForExpression(cdp, `document.body?.innerText.includes(${JSON.stringify(text)})`, text, timeoutMs);
}

async function clickByText(cdp, text) {
  await waitForExpression(
    cdp,
    `[...document.querySelectorAll('button,a')].some((el) => el.textContent?.replace(/\\s+/g, ' ').trim().includes(${JSON.stringify(text)}))`,
    `click target ${text}`,
  );
  await evaluate(
    cdp,
    `(() => {
      const el = [...document.querySelectorAll('button,a')].find((node) => node.textContent?.replace(/\\s+/g, ' ').trim().includes(${JSON.stringify(text)}));
      if (!el) throw new Error('missing click target');
      el.click();
      return true;
    })()`,
  );
}

async function clickByTitle(cdp, title) {
  await waitForExpression(
    cdp,
    `[...document.querySelectorAll('button,a')].some((el) => el.getAttribute('title') === ${JSON.stringify(title)})`,
    `click title ${title}`,
  );
  await evaluate(
    cdp,
    `(() => {
      const el = [...document.querySelectorAll('button,a')].find((node) => node.getAttribute('title') === ${JSON.stringify(title)});
      if (!el) throw new Error('missing titled click target');
      el.click();
      return true;
    })()`,
  );
}

async function clickWithin(cdp, selector, text) {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    if (await evaluate(
      cdp,
      `(() => {
        const root = document.querySelector(${JSON.stringify(selector)});
        const el = root ? [...root.querySelectorAll('button,a')].find((node) => node.textContent?.replace(/\\s+/g, ' ').trim().includes(${JSON.stringify(text)})) : null;
        if (!el) return false;
        el.click();
        return true;
      })()`,
    )) {
      return;
    }
    await sleep(100);
  }
  const debug = await evaluate(
    cdp,
    `(() => {
      const root = document.querySelector(${JSON.stringify(selector)});
      if (!root) return { foundRoot: false };
      return {
        foundRoot: true,
        text: root.textContent?.replace(/\\s+/g, ' ').trim() ?? '',
        buttons: [...root.querySelectorAll('button,a')].map((node) => node.textContent?.replace(/\\s+/g, ' ').trim() ?? ''),
        selects: [...root.querySelectorAll('select')].map((node) => node.value),
        inputs: [...root.querySelectorAll('input')].map((node) => node.value),
      };
    })()`,
  ).catch((error) => ({ error: error instanceof Error ? error.message : String(error) }));
  throw new Error(`timed out waiting for ${text} inside ${selector}: ${JSON.stringify(debug)}`);
}

async function clickGoalAction(cdp, goalTitle, buttonText) {
  await waitForExpression(
    cdp,
    `[...document.querySelectorAll('.goal-item')].some((item) => item.querySelector('h2')?.textContent?.includes(${JSON.stringify(goalTitle)}) && [...item.querySelectorAll('button')].some((button) => button.textContent?.replace(/\\s+/g, ' ').trim().includes(${JSON.stringify(buttonText)})))`,
    `${buttonText} for ${goalTitle}`,
  );
  await evaluate(
    cdp,
    `(() => {
      const item = [...document.querySelectorAll('.goal-item')].find((node) => node.querySelector('h2')?.textContent?.includes(${JSON.stringify(goalTitle)}));
      const button = [...item.querySelectorAll('button')].find((node) => node.textContent?.replace(/\\s+/g, ' ').trim().includes(${JSON.stringify(buttonText)}));
      if (!button) throw new Error('missing goal action');
      button.click();
      return true;
    })()`,
  );
}

async function clickGoalButtonByTitle(cdp, goalTitle, buttonTitle) {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    if (await evaluate(
      cdp,
      `(() => {
        const item = [...document.querySelectorAll('.goal-item')].find((node) => node.querySelector('h2')?.textContent?.includes(${JSON.stringify(goalTitle)}));
        const button = item ? [...item.querySelectorAll('button')].find((node) => node.getAttribute('title') === ${JSON.stringify(buttonTitle)}) : null;
        if (!button) return false;
        button.click();
        return true;
      })()`,
    )) {
      return;
    }
    await sleep(100);
  }
  throw new Error(`timed out waiting for ${buttonTitle} button on ${goalTitle}`);
}

async function waitForGoalText(cdp, goalTitle, text, timeoutMs = 15000) {
  await waitForExpression(
    cdp,
    `[...document.querySelectorAll('.goal-item')].some((item) => item.querySelector('h2')?.textContent?.includes(${JSON.stringify(goalTitle)}) && item.textContent?.includes(${JSON.stringify(text)}))`,
    `${text} for ${goalTitle}`,
    timeoutMs,
  );
}

async function setInputValue(cdp, selector, value) {
  await waitForExpression(cdp, `Boolean(document.querySelector(${JSON.stringify(selector)}))`, `input ${selector}`);
  await evaluate(
    cdp,
    `(() => {
      const input = document.querySelector(${JSON.stringify(selector)});
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setter.call(input, ${JSON.stringify(value)});
      input.dispatchEvent(new Event('input', { bubbles: true }));
      return input.value;
    })()`,
  );
}

async function setSelectValue(cdp, selector, value) {
  await waitForExpression(cdp, `Boolean(document.querySelector(${JSON.stringify(selector)}))`, `select ${selector}`);
  await evaluate(
    cdp,
    `(() => {
      const select = document.querySelector(${JSON.stringify(selector)});
      const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
      setter.call(select, ${JSON.stringify(value)});
      select.dispatchEvent(new Event('change', { bubbles: true }));
      return select.value;
    })()`,
  );
}

async function setSelectValueAt(cdp, scopeSelector, index, value) {
  await waitForExpression(cdp, `Boolean(document.querySelector(${JSON.stringify(scopeSelector)})?.querySelectorAll('select')[${index}])`, `select ${index} in ${scopeSelector}`);
  await evaluate(
    cdp,
    `(() => {
      const select = document.querySelector(${JSON.stringify(scopeSelector)}).querySelectorAll('select')[${index}];
      const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
      setter.call(select, ${JSON.stringify(value)});
      select.dispatchEvent(new Event('change', { bubbles: true }));
      return select.value;
    })()`,
  );
}

async function assertAPIDocsLinks(cdp, expectedBase) {
  const links = await evaluate(
    cdp,
    `[...document.querySelectorAll('a')].filter((link) => link.textContent?.includes('API')).map((link) => link.href)`,
  );
  if (!links.length) throw new Error("landing page did not render API documentation links");
  const bad = links.filter((href) => href !== `${expectedBase}/docs`);
  if (bad.length) throw new Error(`API docs links point at the wrong host: ${bad.join(", ")}`);
}

async function assertVoiceInputSupportedPath(cdp) {
  const before = await evaluate(cdp, `window.__goalstakesVoice?.started ?? 0`);
  await clickByTitle(cdp, "Voice input");
  await waitForText(cdp, "Created the gym goal.");
  await waitForExpression(cdp, `document.querySelector('.chat-composer button[title="Voice input"]') && !document.querySelector('.chat-composer button[title="Voice input"]').disabled`, "voice button reset");
  const state = await evaluate(
    cdp,
    `window.__goalstakesVoice ? ({
      started: window.__goalstakesVoice.started,
      ended: window.__goalstakesVoice.ended,
      transcripts: window.__goalstakesVoice.transcripts,
      languages: window.__goalstakesVoice.languages,
    }) : null`,
  );
  if (!state || state.started <= before || state.ended <= 0 || !state.transcripts.includes("Create a weekly gym goal")) {
    throw new Error(`voice input did not drive a supported transcript path: ${JSON.stringify(state)}`);
  }
}

async function assertVoiceInputUnsupportedPath(cdp) {
  await evaluate(
    cdp,
    `(() => {
      delete window.SpeechRecognition;
      delete window.webkitSpeechRecognition;
      return !window.SpeechRecognition && !window.webkitSpeechRecognition;
    })()`,
  );
  await clickByTitle(cdp, "Voice input");
  await waitForText(cdp, "Voice input is not available in this browser");
}

async function assertLandingHasNoFakeCredentials(cdp) {
  const text = await evaluate(cdp, "document.body.innerText");
  if (/sk_(live|test)/i.test(text) || text.includes("...42a9")) {
    throw new Error(`landing page rendered credential-looking placeholder text: ${text}`);
  }
  if (!text.includes("Backend .env only")) {
    throw new Error("landing page does not state that secrets are backend .env only");
  }
}

function assertChainTokenMatrix(chains) {
  const expected = {
    ethereum: { enforcer: ethereumEnforcer, tokens: { USDC: ethereumUSDC, USDT: ethereumUSDT } },
    polygon: { enforcer: polygonEnforcer, tokens: { USDC: polygonUSDC, USDT: polygonUSDT } },
  };
  for (const [chainKey, want] of Object.entries(expected)) {
    const chain = chains.find((item) => item.key === chainKey);
    if (!chain) throw new Error(`GET /api/v1/chains missing ${chainKey}: ${JSON.stringify(chains)}`);
    if (String(chain.stake_enforcer_address).toLowerCase() !== want.enforcer.toLowerCase()) {
      throw new Error(`${chainKey} StakeEnforcer = ${chain.stake_enforcer_address}, want ${want.enforcer}`);
    }
    for (const [symbol, address] of Object.entries(want.tokens)) {
      if (String(chain.tokens?.[symbol] ?? "").toLowerCase() !== address.toLowerCase()) {
        throw new Error(`${chainKey} ${symbol} = ${chain.tokens?.[symbol]}, want ${address}`);
      }
    }
  }
}

async function walletTransactionCount(cdp) {
  return Number(await evaluate(cdp, `window.__goalstakesWalletEvents?.transactions?.length ?? 0`));
}

async function approveAndAssertMatrixCombo(cdp, sessionToken, { chain, tokenSymbol, chainID, tokenAddress, spender }) {
  await setSelectValueAt(cdp, ".approval-panel", 0, chain);
  await setSelectValueAt(cdp, ".approval-panel", 1, tokenSymbol);
  const before = await walletTransactionCount(cdp);
  await clickWithin(cdp, ".approval-panel", "Approve");
  await assertApprovalTx(cdp, {
    chainID,
    token: tokenAddress,
    spender,
    amount: "100000000",
    afterTransactionCount: before,
  });
  const approval = await waitForRecordedApproval(sessionToken, chain, tokenSymbol, "100000000");
  if (approval.allowance !== "100000000") {
    throw new Error(`${chain} ${tokenSymbol} recorded approval allowance = ${approval.allowance}, want 100000000`);
  }
}

async function assertRecordedApprovalMatrix(sessionToken) {
  for (const [chain, tokenSymbol] of [
    ["ethereum", "USDC"],
    ["ethereum", "USDT"],
    ["polygon", "USDC"],
    ["polygon", "USDT"],
  ]) {
    const approval = await waitForRecordedApproval(sessionToken, chain, tokenSymbol, "100000000");
    if (approval.allowance !== "100000000") {
      throw new Error(`${chain} ${tokenSymbol} allowance = ${approval.allowance}, want 100000000`);
    }
  }
}

async function waitForRecordedApproval(sessionToken, chain, tokenSymbol, expectedAllowance, timeoutMs = 5000) {
  const deadline = Date.now() + timeoutMs;
  let last = "";
  while (Date.now() < deadline) {
    const approval = await requestJSON(`${apiBase}/api/v1/approvals?chain=${chain}&token_symbol=${tokenSymbol}`, {
      headers: { Authorization: `Bearer ${sessionToken}` },
    });
    if (approval.allowance === expectedAllowance) return approval;
    last = approval.allowance;
    await sleep(100);
  }
  throw new Error(`${chain} ${tokenSymbol} allowance stayed ${last || "unavailable"}, want ${expectedAllowance}`);
}

function localDateInput(value) {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  return `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, "0")}-${String(parsed.getDate()).padStart(2, "0")}`;
}

async function assertApprovalTx(cdp, { chainID, token, spender, amount, afterTransactionCount = -1 }) {
  await waitForExpression(
    cdp,
    `(() => {
      const events = window.__goalstakesWalletEvents;
      if (!events) return false;
      const switches = (events.switches ?? []).map((value) => String(value).toLowerCase());
      const txs = events.transactions ?? [];
      return txs.length > ${Number(afterTransactionCount)}
        && switches.includes(${JSON.stringify(chainID.toLowerCase())})
        && txs.slice(${Math.max(0, afterTransactionCount)}).some((item) => String(item.to ?? "").toLowerCase() === ${JSON.stringify(token.toLowerCase())});
    })()`,
    `approval transaction to ${token}`,
  );
  const raw = await evaluate(cdp, `JSON.stringify(window.__goalstakesWalletEvents)`);
  const events = JSON.parse(raw);
  const switches = events.switches ?? [];
  if (!switches.map((value) => value.toLowerCase()).includes(chainID.toLowerCase())) {
    throw new Error(`wallet did not switch to ${chainID}; switches=${JSON.stringify(switches)}`);
  }
  const candidates = afterTransactionCount >= 0 ? (events.transactions ?? []).slice(afterTransactionCount) : events.transactions ?? [];
  const tx = [...candidates].reverse().find((item) => String(item.to ?? "").toLowerCase() === token.toLowerCase());
  if (!tx) {
    throw new Error(`no approval transaction sent to token ${token}; transactions=${JSON.stringify(events.transactions)}`);
  }
  const data = String(tx.data ?? "").toLowerCase();
  const encodedSpender = spender.toLowerCase().replace(/^0x/, "").padStart(64, "0");
  const encodedAmount = BigInt(amount).toString(16).padStart(64, "0");
  if (!data.startsWith("0x095ea7b3") || !data.includes(encodedSpender) || !data.includes(encodedAmount)) {
    throw new Error(`approval calldata did not target spender/amount: ${data}`);
  }
}

function ethereumMockScript(address, signerURL) {
  return `
(() => {
  const address = ${JSON.stringify(address)};
  const signerURL = ${JSON.stringify(signerURL)};
  let chainId = "0xaa36a7";
  let txCounter = 1n;
  const receipts = new Map();
  const listeners = new Map();
  const walletEvents = window.__goalstakesWalletEvents = {
    switches: [],
    transactions: [],
    rejectNextSignature: false,
    revertNextReceipt: false,
  };
  const hex = (value, length) => "0x" + value.toString(16).padStart(length, "0");
  window.ethereum = {
    isMetaMask: true,
    on(event, handler) {
      const next = listeners.get(event) || [];
      next.push(handler);
      listeners.set(event, next);
    },
    removeListener(event, handler) {
      listeners.set(event, (listeners.get(event) || []).filter((item) => item !== handler));
    },
    async request(payload) {
      const method = payload?.method;
      const params = payload?.params || [];
      if (method === "eth_requestAccounts" || method === "eth_accounts") return [address];
      if (method === "personal_sign") {
        if (walletEvents.rejectNextSignature) {
          walletEvents.rejectNextSignature = false;
          throw new Error("User rejected signing");
        }
        const response = await fetch(signerURL + "/sign", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ message: params[0], address: params[1] || address }),
        });
        const body = await response.json();
        if (!response.ok) throw new Error(body.error || "mock signature failed");
        return body.signature;
      }
      if (method === "eth_chainId") return chainId;
      if (method === "wallet_switchEthereumChain") {
        chainId = params[0]?.chainId || chainId;
        walletEvents.switches.push(chainId);
        (listeners.get("chainChanged") || []).forEach((handler) => handler(chainId));
        return null;
      }
      if (method === "eth_call") return hex(0n, 64);
      if (method === "eth_blockNumber") return "0x1";
      if (method === "eth_estimateGas") return "0x5208";
      if (method === "eth_gasPrice" || method === "eth_maxPriorityFeePerGas") return "0x1";
      if (method === "eth_sendTransaction") {
        const hash = hex(txCounter++, 64);
        const status = walletEvents.revertNextReceipt ? "0x0" : "0x1";
        walletEvents.revertNextReceipt = false;
        walletEvents.transactions.push(params[0] || {});
        receipts.set(hash, {
          transactionHash: hash,
          transactionIndex: "0x0",
          blockHash: hex(1n, 64),
          blockNumber: "0x1",
          from: address,
          to: params[0]?.to || null,
          cumulativeGasUsed: "0x5208",
          gasUsed: "0x5208",
          effectiveGasPrice: "0x1",
          contractAddress: null,
          logs: [],
          logsBloom: "0x" + "0".repeat(512),
          status,
          type: "0x2",
        });
        return hash;
      }
      if (method === "eth_getTransactionReceipt") return receipts.get(params[0]) || null;
      if (method === "eth_getTransactionByHash") {
        const receipt = receipts.get(params[0]);
        return receipt ? { hash: receipt.transactionHash, from: receipt.from, to: receipt.to, blockNumber: receipt.blockNumber, transactionIndex: "0x0", value: "0x0", input: "0x", gas: "0x5208", gasPrice: "0x1" } : null;
      }
      if (method === "net_version") return String(Number.parseInt(chainId, 16));
      throw new Error("unhandled ethereum mock method: " + method);
    },
  };
})();`;
}

function voiceRecognitionMockScript(transcript) {
  return `
(() => {
  const transcript = ${JSON.stringify(transcript)};
  const state = window.__goalstakesVoice = {
    started: 0,
    ended: 0,
    transcripts: [],
    languages: [],
  };
  class GoalStakesSpeechRecognition {
    constructor() {
      this.lang = "";
      this.interimResults = true;
      this.maxAlternatives = 0;
      this.onresult = null;
      this.onerror = null;
      this.onend = null;
    }
    start() {
      state.started += 1;
      state.languages.push(this.lang);
      setTimeout(() => {
        state.transcripts.push(transcript);
        if (this.onresult) this.onresult({ results: [[{ transcript }]] });
        state.ended += 1;
        if (this.onend) this.onend();
      }, 25);
    }
    stop() {
      state.ended += 1;
      if (this.onend) this.onend();
    }
  }
  Object.defineProperty(window, "SpeechRecognition", { configurable: true, writable: true, value: GoalStakesSpeechRecognition });
  Object.defineProperty(window, "webkitSpeechRecognition", { configurable: true, writable: true, value: GoalStakesSpeechRecognition });
})();`;
}

function staleWalletStorageScript(origin) {
  return `
(() => {
  if (location.origin !== ${JSON.stringify(origin)}) return;
  if (sessionStorage.getItem("__goalstakes_stale_wallet_seeded") === "1") return;
  sessionStorage.setItem("__goalstakes_stale_wallet_seeded", "1");
  localStorage.removeItem("goalstakes.session");
  localStorage.setItem("goalstakes.wallet", "0x3333333333333333333333333333333333333333");
})();`;
}

function closeServer(server) {
  return new Promise((resolve) => server.close(resolve));
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().then(() => {
  process.exit(0);
}).catch((error) => {
  console.error(error instanceof Error ? error.stack : error);
  process.exit(1);
});
