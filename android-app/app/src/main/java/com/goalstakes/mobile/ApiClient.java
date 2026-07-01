package com.goalstakes.mobile;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.TimeUnit;

import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

public final class ApiClient {
    private static final MediaType JSON = MediaType.get("application/json; charset=utf-8");

    private final String baseUrl;
    private final String apiKey;
    private final OkHttpClient http;

    public ApiClient(String baseUrl, String apiKey) {
        this.baseUrl = trimTrailingSlash(baseUrl);
        this.apiKey = apiKey == null ? "" : apiKey.trim();
        this.http = new OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .build();
    }

    public List<Goal> listGoals() throws Exception {
        JSONArray rows = new JSONArray(request("GET", "/api/v1/goals", null));
        List<Goal> goals = new ArrayList<>();
        for (int i = 0; i < rows.length(); i++) {
            goals.add(goalFromJSON(rows.getJSONObject(i)));
        }
        return goals;
    }

    public Goal createGoal(String title, String type, String cadence, String stake, String token, String chain) throws Exception {
        return createGoal(title, type, cadence, stake, token, chain, null, null);
    }

    public Goal createGoal(String title, String type, String cadence, String stake, String token, String chain, String startsAt, String endsAt) throws Exception {
        JSONObject body = new JSONObject()
                .put("title", title)
                .put("type", type)
                .put("cadence", cadence)
                .put("stake_amount", toBaseUnits(stake))
                .put("token_symbol", token.toUpperCase(Locale.US))
                .put("chain", chain)
                .put("timezone", java.util.TimeZone.getDefault().getID());
        putOptionalTimestamp(body, "starts_at", startsAt);
        putOptionalTimestamp(body, "ends_at", endsAt);
        return goalFromJSON(new JSONObject(request("POST", "/api/v1/goals", body)));
    }

    public Goal updateGoal(String goalId, String title, String description, String stake) throws Exception {
        return updateGoal(goalId, title, description, stake, null);
    }

    public Goal updateGoal(String goalId, String title, String description, String stake, String endsAt) throws Exception {
        JSONObject body = new JSONObject()
                .put("title", title)
                .put("description", description == null ? "" : description)
                .put("stake_amount", toBaseUnits(stake));
        putEndTimestamp(body, endsAt);
        return goalFromJSON(new JSONObject(request("PATCH", "/api/v1/goals/" + goalId, body)));
    }

    public Goal updateStake(String goalId, String stake, String token, String chain) throws Exception {
        JSONObject body = new JSONObject()
                .put("stake_amount", toBaseUnits(stake))
                .put("token_symbol", token.toUpperCase(Locale.US))
                .put("chain", chain);
        return goalFromJSON(new JSONObject(request("PATCH", "/api/v1/goals/" + goalId + "/stake", body)));
    }

    public void archiveGoal(String goalId) throws Exception {
        request("DELETE", "/api/v1/goals/" + goalId, null);
    }

    public void logCheckIn(String goalId, String note) throws Exception {
        request("POST", "/api/v1/goals/" + goalId + "/checkins", new JSONObject().put("note", note));
    }

    public void reportViolation(String goalId, String reason) throws Exception {
        request("POST", "/api/v1/goals/" + goalId + "/violations", new JSONObject().put("reason", reason));
    }

    public Progress getProgress(String goalId) throws Exception {
        JSONObject raw = new JSONObject(request("GET", "/api/v1/goals/" + goalId + "/progress", null));
        JSONArray violations = raw.optJSONArray("violations");
        return new Progress(
                goalFromJSON(raw.getJSONObject("goal")),
                raw.optString("current_period"),
                raw.optBoolean("current_period_completed"),
                violations == null ? 0 : violations.length()
        );
    }

    public String chat(String message) throws Exception {
        JSONObject response = new JSONObject(request("POST", "/api/v1/chat", new JSONObject().put("message", message)));
        return response.optString("reply", "");
    }

    public String createAgentLink(String name) throws Exception {
        JSONObject response = new JSONObject(request("POST", "/api/v1/agent-links", new JSONObject().put("name", name == null ? "" : name)));
        return response.optString("skill_url", "");
    }

    String request(String method, String path, JSONObject body) throws Exception {
        RequestBody requestBody = body == null ? null : RequestBody.create(body.toString(), JSON);
        Request.Builder builder = new Request.Builder()
                .url(baseUrl + path)
                .header("Accept", "application/json")
                .header("Authorization", "Bearer " + apiKey);
        if ("GET".equals(method)) {
            builder.get();
        } else if ("DELETE".equals(method) && requestBody == null) {
            builder.delete();
        } else {
            builder.method(method, requestBody);
        }
        try (Response response = http.newCall(builder.build()).execute()) {
            String responseBody = response.body() == null ? "" : response.body().string();
            if (!response.isSuccessful()) {
                String error = responseBody;
                try {
                    error = new JSONObject(responseBody).optString("error", responseBody);
                } catch (Exception ignored) {
                    // Keep raw response.
                }
                throw new IllegalStateException(error.isEmpty() ? "HTTP " + response.code() : error);
            }
            return responseBody;
        }
    }

    private static Goal goalFromJSON(JSONObject raw) {
        return new Goal(
                raw.optString("id"),
                raw.optString("title"),
                raw.optString("type"),
                raw.optString("cadence"),
                raw.optString("stake_amount"),
                raw.optString("token_symbol"),
                raw.optString("chain"),
                raw.optString("starts_at", ""),
                raw.optString("ends_at", "")
        );
    }

    static String toBaseUnits(String decimal) {
        String value = decimal == null || decimal.trim().isEmpty() ? "0" : decimal.trim();
        String[] parts = value.split("\\.", -1);
        String whole = parts.length > 0 && !parts[0].isEmpty() ? parts[0] : "0";
        String frac = parts.length > 1 ? parts[1] : "";
        if (parts.length > 2 || !whole.matches("\\d+") || !frac.matches("\\d*")) {
            throw new IllegalArgumentException("Stake must be a decimal number");
        }
        if (frac.length() > 6) {
            throw new IllegalArgumentException("USDC/USDT stakes support up to 6 decimals");
        }
        String padded = (frac + "000000").substring(0, 6);
        String combined = (whole + padded).replaceFirst("^0+(?!$)", "");
        return combined.isEmpty() ? "0" : combined;
    }

    private static String trimTrailingSlash(String raw) {
        String value = raw == null || raw.trim().isEmpty() ? "http://10.0.2.2:8080" : raw.trim();
        if (!value.startsWith("http://") && !value.startsWith("https://")) {
            throw new IllegalArgumentException("API URL must start with http:// or https://");
        }
        while (value.endsWith("/")) {
            value = value.substring(0, value.length() - 1);
        }
        return value;
    }

    private static void putOptionalTimestamp(JSONObject body, String key, String value) throws Exception {
        String normalized = normalizeTimestamp(value);
        if (!normalized.isEmpty()) {
            body.put(key, normalized);
        }
    }

    private static void putEndTimestamp(JSONObject body, String value) throws Exception {
        if (value == null) {
            return;
        }
        String normalized = normalizeTimestamp(value);
        body.put("ends_at", normalized.isEmpty() ? JSONObject.NULL : normalized);
    }

    private static String normalizeTimestamp(String value) {
        String trimmed = value == null ? "" : value.trim();
        if (trimmed.matches("\\d{4}-\\d{2}-\\d{2}")) {
            return trimmed + "T00:00:00Z";
        }
        return trimmed;
    }
}
