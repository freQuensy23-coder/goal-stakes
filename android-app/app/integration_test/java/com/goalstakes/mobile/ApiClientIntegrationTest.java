package com.goalstakes.mobile;

import org.junit.Assume;
import org.junit.Test;

import java.time.Instant;
import java.util.List;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

public final class ApiClientIntegrationTest {
    @Test
    public void publicApiKeyCanCreateGoalChatAndCheckInAgainstBackend() throws Exception {
        String baseUrl = System.getProperty("goalstakes.e2e.baseUrl", "");
        String apiKey = System.getProperty("goalstakes.e2e.apiKey", "");
        String title = System.getProperty("goalstakes.e2e.goalTitle", "Android API e2e");
        String chain = System.getProperty("goalstakes.e2e.chain", "sepolia");
        String expectedTitles = System.getProperty("goalstakes.e2e.expectedTitles", "");
        Assume.assumeTrue("goalstakes.e2e.baseUrl is required", !baseUrl.isEmpty());
        Assume.assumeTrue("goalstakes.e2e.apiKey is required", apiKey.startsWith("sk_"));

        ApiClient client = new ApiClient(baseUrl, apiKey);

        List<Goal> initial = client.listGoals();
        assertFalse("integration setup should create at least one goal before Android runs", initial.isEmpty());
        for (String expectedTitle : expectedTitles.split("\\|", -1)) {
            if (!expectedTitle.trim().isEmpty()) {
                assertGoalTitleVisible(initial, expectedTitle.trim());
            }
        }

        Goal created = client.createGoal(title, "do", "daily", "1", "USDC", chain, "2026-05-25", "2026-06-05");
        assertEquals(title, created.title);
        assertEquals("1000000", created.stakeAmount);
        assertSameInstant("2026-05-25T00:00:00Z", created.startsAt);
        assertSameInstant("2026-06-05T00:00:00Z", created.endsAt);

        Goal updated = client.updateGoal(created.id, title + " updated", "updated from Android API integration", "1.25", "2026-06-10");
        assertEquals(title + " updated", updated.title);
        assertEquals("1250000", updated.stakeAmount);
        assertSameInstant("2026-06-10T00:00:00Z", updated.endsAt);

        Goal cleared = client.updateGoal(created.id, title + " updated", "cleared end date from Android API integration", "1.25", "");
        assertEquals("", cleared.endsAt);

        Goal restaked = client.updateStake(created.id, "2", "USDT", "polygon");
        assertEquals("2000000", restaked.stakeAmount);
        assertEquals("USDT", restaked.tokenSymbol);
        assertEquals("polygon", restaked.chain);

        client.logCheckIn(created.id, "checked in from Android API integration");
        Progress progress = client.getProgress(created.id);
        assertTrue("check-in should complete the current period", progress.currentPeriodCompleted);
        assertEquals(created.id, progress.goal.id);

        client.archiveGoal(created.id);
        List<Goal> afterArchive = client.listGoals();
        for (Goal goal : afterArchive) {
            assertFalse("archived Android goal should be omitted from active goals", created.id.equals(goal.id));
        }

        Goal visible = client.createGoal(title + " visible", "avoid", "daily", "1", "USDC", chain);
        assertEquals(title + " visible", visible.title);
        assertEquals("1000000", visible.stakeAmount);
        boolean foundVisible = false;
        List<Goal> afterVisibleCreate = client.listGoals();
        for (Goal goal : afterVisibleCreate) {
            if (visible.id.equals(goal.id)) {
                foundVisible = true;
                break;
            }
        }
        assertTrue("visible Android-created goal should remain active for frontend verification", foundVisible);

        String reply = client.chat("Create another weekly gym goal with a $100 stake");
        assertTrue("chat reply should come from the AI manager", reply.contains("Created"));
    }

    private static void assertSameInstant(String expected, String actual) {
        assertEquals(Instant.parse(expected), Instant.parse(actual));
    }

    private static void assertGoalTitleVisible(List<Goal> goals, String title) {
        for (Goal goal : goals) {
            if (title.equals(goal.title)) {
                return;
            }
        }
        throw new AssertionError("expected Android API client to see active goal title: " + title);
    }
}
