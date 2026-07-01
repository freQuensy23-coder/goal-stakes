package com.goalstakes.mobile;

import org.junit.After;
import org.junit.Before;
import org.junit.Test;

import java.io.ByteArrayOutputStream;
import java.io.OutputStream;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

public final class ApiClientTest {
    private ServerSocket server;
    private ExecutorService executor;
    private String lastMethod;
    private String lastPath;
    private String lastAuth;
    private String lastBody;
    private volatile int nextStatus;
    private volatile String nextBody;

    @Before
    public void startServer() throws Exception {
        server = new ServerSocket(0);
        nextStatus = 200;
        nextBody = null;
        executor = Executors.newSingleThreadExecutor();
        executor.execute(this::serveOneRequest);
    }

    @After
    public void stopServer() throws Exception {
        server.close();
        executor.shutdownNow();
    }

    @Test
    public void listGoalsUsesBearerApiKey() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        List<Goal> goals = client.listGoals();

        assertEquals("GET", lastMethod);
        assertEquals("/api/v1/goals", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertEquals(1, goals.size());
        assertEquals("Do push-ups", goals.get(0).title);
    }

    @Test
    public void createGoalSendsBaseUnitStake() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.createGoal("Gym", "do", "weekly", "2.50", "usdc", "polygon");

        assertEquals("POST", lastMethod);
        assertEquals("/api/v1/goals", lastPath);
        assertTrue(lastBody.contains("\"stake_amount\":\"2500000\""));
        assertTrue(lastBody.contains("\"token_symbol\":\"USDC\""));
        assertTrue(lastBody.contains("\"chain\":\"polygon\""));
    }

    @Test
    public void createGoalSendsScheduleFieldsWhenProvided() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        Goal goal = client.createGoal(
                "Gym",
                "do",
                "weekly",
                "2.50",
                "usdc",
                "polygon",
                "2026-05-25T00:00:00Z",
                "2026-06-05T00:00:00Z"
        );

        assertEquals("POST", lastMethod);
        assertEquals("/api/v1/goals", lastPath);
        assertTrue(lastBody.contains("\"starts_at\":\"2026-05-25T00:00:00Z\""));
        assertTrue(lastBody.contains("\"ends_at\":\"2026-06-05T00:00:00Z\""));
        assertEquals("2026-05-25T00:00:00Z", goal.startsAt);
        assertEquals("2026-06-05T00:00:00Z", goal.endsAt);
    }

    @Test
    public void createGoalNormalizesDateOnlyScheduleInputs() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.createGoal("Gym", "do", "weekly", "2.50", "usdc", "polygon", "2026-05-25", "2026-06-05");

        assertTrue(lastBody.contains("\"starts_at\":\"2026-05-25T00:00:00Z\""));
        assertTrue(lastBody.contains("\"ends_at\":\"2026-06-05T00:00:00Z\""));
    }

    @Test
    public void listGoalsParsesScheduleFields() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        Goal goal = client.listGoals().get(0);

        assertEquals("2026-05-25T00:00:00Z", goal.startsAt);
        assertEquals("2026-06-05T00:00:00Z", goal.endsAt);
    }

    @Test
    public void getProgressParsesCompletionAndViolationCount() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        Progress progress = client.getProgress("goal-1");

        assertEquals("goal-1", progress.goal.id);
        assertEquals("2026-05-26", progress.currentPeriod);
        assertTrue(progress.currentPeriodCompleted);
        assertEquals(2, progress.violationCount);
    }

    @Test
    public void reportViolationUsesBearerApiKeyAndReasonBody() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.reportViolation("goal-1", "drank soda");

        assertEquals("POST", lastMethod);
        assertEquals("/api/v1/goals/goal-1/violations", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertTrue(lastBody.contains("\"reason\":\"drank soda\""));
    }

    @Test
    public void logCheckInUsesBearerApiKeyAndNoteBody() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.logCheckIn("goal-1", "finished today");

        assertEquals("POST", lastMethod);
        assertEquals("/api/v1/goals/goal-1/checkins", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertTrue(lastBody.contains("\"note\":\"finished today\""));
    }

    @Test
    public void chatPostsMessageAndReturnsReply() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        String reply = client.chat("Create a weekly gym goal");

        assertEquals("POST", lastMethod);
        assertEquals("/api/v1/chat", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertTrue(lastBody.contains("\"message\":\"Create a weekly gym goal\""));
        assertEquals("Recorded.", reply);
    }

    @Test
    public void updateGoalPatchesTitleDescriptionAndStake() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        Goal goal = client.updateGoal("goal-1", "Do 120 push-ups", "harder", "2.50");

        assertEquals("PATCH", lastMethod);
        assertEquals("/api/v1/goals/goal-1", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertTrue(lastBody.contains("\"title\":\"Do 120 push-ups\""));
        assertTrue(lastBody.contains("\"description\":\"harder\""));
        assertTrue(lastBody.contains("\"stake_amount\":\"2500000\""));
        assertEquals("Do 120 push-ups", goal.title);
    }

    @Test
    public void updateGoalSendsEndDateWhenProvided() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        Goal goal = client.updateGoal("goal-1", "Do 120 push-ups", "harder", "2.50", "2026-06-10T00:00:00Z");

        assertEquals("PATCH", lastMethod);
        assertEquals("/api/v1/goals/goal-1", lastPath);
        assertTrue(lastBody.contains("\"ends_at\":\"2026-06-10T00:00:00Z\""));
        assertEquals("2026-06-10T00:00:00Z", goal.endsAt);
    }

    @Test
    public void updateGoalNormalizesDateOnlyEndDate() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.updateGoal("goal-1", "Do 120 push-ups", "harder", "2.50", "2026-06-10");

        assertEquals("PATCH", lastMethod);
        assertEquals("/api/v1/goals/goal-1", lastPath);
        assertTrue(lastBody.contains("\"ends_at\":\"2026-06-10T00:00:00Z\""));
    }

    @Test
    public void updateGoalClearsEndDateWhenEmptyStringProvided() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.updateGoal("goal-1", "Do 120 push-ups", "harder", "2.50", "");

        assertEquals("PATCH", lastMethod);
        assertEquals("/api/v1/goals/goal-1", lastPath);
        assertTrue(lastBody.contains("\"ends_at\":null"));
    }

    @Test
    public void updateStakePatchesTokenAndChain() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        Goal goal = client.updateStake("goal-1", "3", "usdt", "polygon");

        assertEquals("PATCH", lastMethod);
        assertEquals("/api/v1/goals/goal-1/stake", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertTrue(lastBody.contains("\"stake_amount\":\"3000000\""));
        assertTrue(lastBody.contains("\"token_symbol\":\"USDT\""));
        assertTrue(lastBody.contains("\"chain\":\"polygon\""));
        assertEquals("USDT", goal.tokenSymbol);
        assertEquals("polygon", goal.chain);
    }

    @Test
    public void archiveGoalSendsDelete() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        client.archiveGoal("goal-1");

        assertEquals("DELETE", lastMethod);
        assertEquals("/api/v1/goals/goal-1", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
    }

    @Test
    public void createAgentLinkParsesSkillUrl() throws Exception {
        ApiClient client = new ApiClient(baseUrl(), "sk_test");

        String skillUrl = client.createAgentLink("android");

        assertEquals("POST", lastMethod);
        assertEquals("/api/v1/agent-links", lastPath);
        assertEquals("Bearer sk_test", lastAuth);
        assertTrue(lastBody.contains("\"name\":\"android\""));
        assertEquals("https://api.goalstakes.test/agent-skills/agt_private.md", skillUrl);
    }

    @Test
    public void convertsDecimalStakesToSixDecimals() {
        assertEquals("100000000", ApiClient.toBaseUnits("100"));
        assertEquals("2500000", ApiClient.toBaseUnits("2.5"));
        assertEquals("1", ApiClient.toBaseUnits("0.000001"));
    }

    @Test
    public void rejectsStakeDecimalsBeyondSixPlaces() {
        try {
            ApiClient.toBaseUnits("1.0000001");
        } catch (IllegalArgumentException error) {
            assertTrue(error.getMessage().contains("up to 6 decimals"));
            return;
        }
        throw new AssertionError("expected stake precision error");
    }

    @Test
    public void rejectsInvalidApiUrlWithReadableMessage() throws Exception {
        try {
            new ApiClient("not a url", "sk_test");
        } catch (IllegalArgumentException error) {
            assertEquals("API URL must start with http:// or https://", error.getMessage());
            return;
        }
        throw new AssertionError("expected API URL validation error");
    }

    @Test
    public void surfacesStructuredApiErrors() throws Exception {
        nextStatus = 401;
        nextBody = "{\"error\":\"Unauthorized\"}";
        ApiClient client = new ApiClient(baseUrl(), "sk_revoked");

        try {
            client.listGoals();
        } catch (IllegalStateException error) {
            assertEquals("Unauthorized", error.getMessage());
            return;
        }
        throw new AssertionError("expected structured API error");
    }

    private String baseUrl() {
        return "http://127.0.0.1:" + server.getLocalPort();
    }

    private static String responseFor(String method, String path) {
        if ("GET".equals(method) && "/api/v1/goals".equals(path)) {
            return "[{\"id\":\"goal-1\",\"title\":\"Do push-ups\",\"type\":\"do\",\"cadence\":\"daily\",\"stake_amount\":\"1000000\",\"token_symbol\":\"USDC\",\"chain\":\"sepolia\",\"starts_at\":\"2026-05-25T00:00:00Z\",\"ends_at\":\"2026-06-05T00:00:00Z\"}]";
        }
        if ("GET".equals(method) && "/api/v1/goals/goal-1/progress".equals(path)) {
            return "{\"goal\":{\"id\":\"goal-1\",\"title\":\"Do push-ups\",\"type\":\"do\",\"cadence\":\"daily\",\"stake_amount\":\"1000000\",\"token_symbol\":\"USDC\",\"chain\":\"sepolia\"},\"current_period\":\"2026-05-26\",\"current_period_completed\":true,\"violations\":[{\"id\":\"v1\"},{\"id\":\"v2\"}]}";
        }
        if ("/api/v1/chat".equals(path)) {
            return "{\"conversation_id\":\"00000000-0000-0000-0000-000000000001\",\"reply\":\"Recorded.\"}";
        }
        if ("POST".equals(method) && "/api/v1/agent-links".equals(path)) {
            return "{\"skill_url\":\"https://api.goalstakes.test/agent-skills/agt_private.md\",\"agent_link\":{\"id\":\"link-1\",\"api_key_id\":\"key-1\",\"name\":\"android\",\"expires_at\":\"2026-09-29T00:00:00Z\",\"created_at\":\"2026-07-01T00:00:00Z\"}}";
        }
        if ("PATCH".equals(method) && "/api/v1/goals/goal-1".equals(path)) {
            return "{\"id\":\"goal-1\",\"title\":\"Do 120 push-ups\",\"type\":\"do\",\"cadence\":\"daily\",\"stake_amount\":\"2500000\",\"token_symbol\":\"USDC\",\"chain\":\"sepolia\",\"starts_at\":\"2026-05-25T00:00:00Z\",\"ends_at\":\"2026-06-10T00:00:00Z\"}";
        }
        if ("PATCH".equals(method) && "/api/v1/goals/goal-1/stake".equals(path)) {
            return "{\"id\":\"goal-1\",\"title\":\"Do push-ups\",\"type\":\"do\",\"cadence\":\"daily\",\"stake_amount\":\"3000000\",\"token_symbol\":\"USDT\",\"chain\":\"polygon\"}";
        }
        return "{\"id\":\"goal-2\",\"title\":\"Gym\",\"type\":\"do\",\"cadence\":\"weekly\",\"stake_amount\":\"2500000\",\"token_symbol\":\"USDC\",\"chain\":\"polygon\",\"starts_at\":\"2026-05-25T00:00:00Z\",\"ends_at\":\"2026-06-05T00:00:00Z\"}";
    }

    private void serveOneRequest() {
        try (Socket socket = server.accept()) {
            socket.setSoTimeout(3000);
            ByteArrayOutputStream headerBytes = new ByteArrayOutputStream();
            int b;
            while ((b = socket.getInputStream().read()) != -1) {
                headerBytes.write(b);
                String candidate = headerBytes.toString(StandardCharsets.UTF_8);
                if (candidate.contains("\r\n\r\n")) break;
            }
            String headersOnly = headerBytes.toString(StandardCharsets.UTF_8);
            String[] parts = headersOnly.split("\r\n\r\n", 2);
            String[] lines = parts[0].split("\\r?\\n");
            String[] requestLine = lines[0].split(" ");
            lastMethod = requestLine[0];
            lastPath = requestLine[1];
            int contentLength = 0;
            for (String line : lines) {
                if (line.toLowerCase().startsWith("authorization:")) {
                    lastAuth = line.substring(line.indexOf(":") + 1).trim();
                }
                if (line.toLowerCase().startsWith("content-length:")) {
                    contentLength = Integer.parseInt(line.substring(line.indexOf(":") + 1).trim());
                }
            }
            byte[] bodyBytes = socket.getInputStream().readNBytes(contentLength);
            lastBody = new String(bodyBytes, StandardCharsets.UTF_8);
            int status = nextStatus;
            String body = nextBody == null ? responseFor(lastMethod, lastPath) : nextBody;
            byte[] payload = body.getBytes(StandardCharsets.UTF_8);
            String statusText = status == 200 ? "OK" : "Error";
            String headers = "HTTP/1.1 " + status + " " + statusText + "\r\nContent-Type: application/json\r\nContent-Length: " + payload.length + "\r\nConnection: close\r\n\r\n";
            OutputStream out = socket.getOutputStream();
            out.write(headers.getBytes(StandardCharsets.UTF_8));
            out.write(payload);
            out.flush();
        } catch (Exception ignored) {
            // Test teardown closes the socket.
        }
    }
}
