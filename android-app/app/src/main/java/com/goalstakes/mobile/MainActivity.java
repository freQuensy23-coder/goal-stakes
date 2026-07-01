package com.goalstakes.mobile;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.res.Configuration;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.speech.RecognizerIntent;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.ArrayAdapter;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.Spinner;
import android.widget.TextView;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final String PREFS = "goalstakes";
    private static final int REQUEST_VOICE_CHAT = 1001;

    private static final int COLOR_BG = Color.rgb(246, 248, 245);
    private static final int COLOR_SURFACE = Color.WHITE;
    private static final int COLOR_TEXT = Color.rgb(25, 32, 45);
    private static final int COLOR_MUTED = Color.rgb(89, 100, 116);
    private static final int COLOR_BORDER = Color.rgb(215, 222, 226);
    private static final int COLOR_GREEN = Color.rgb(20, 113, 76);
    private static final int COLOR_BLUE = Color.rgb(35, 82, 139);
    private static final int COLOR_RED = Color.rgb(177, 47, 39);
    private static final int COLOR_AMBER = Color.rgb(150, 91, 24);

    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final List<Goal> goals = new ArrayList<>();

    private LinearLayout root;
    private LinearLayout tabBar;
    private LinearLayout content;
    private LinearLayout goalList;
    private LinearLayout actionPanel;
    private LinearLayout formPanel;
    private TextView statusLine;
    private EditText chatMessageInput;
    private TextView chatReplyOutput;
    private String currentScreen = "goals";
    private String selectedGoalId = "";
    private boolean showCreateForm;
    private boolean showEditForm;
    private boolean compactLayout;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        buildShell();
        showGoals();
    }

    @Override
    protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }

    private void buildShell() {
        compactLayout = getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
        root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(COLOR_BG);
        root.setPadding(dp(16), compactLayout ? dp(8) : dp(18), dp(16), dp(10));
        setContentView(root);

        TextView title = text("Goal Stakes", compactLayout ? 22 : 28, true);
        title.setPadding(0, dp(4), 0, dp(2));
        root.addView(title);
        root.addView(body("Crypto-backed commitments without custody. Miss a goal and the stake burns."));

        statusLine = pill("Ready", COLOR_BLUE, Color.rgb(233, 241, 250));
        addWithMargins(root, statusLine, 0, compactLayout ? dp(6) : dp(12), 0, compactLayout ? dp(4) : dp(8));
        root.addView(statusLine);

        tabBar = row();
        tabBar.setPadding(0, 0, 0, compactLayout ? dp(6) : dp(10));
        root.addView(tabBar);

        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(false);
        content = column();
        content.setPadding(0, 0, 0, dp(28));
        scroll.addView(content, new ScrollView.LayoutParams(-1, -2));
        root.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
    }

    private void renderTabs() {
        tabBar.removeAllViews();
        tabBar.addView(tab("Goals", "goals", () -> showGoals()));
        tabBar.addView(tab("Chat", "chat", () -> showChat()));
        tabBar.addView(tab("Settings", "settings", () -> showSettings()));
    }

    private void showGoals() {
        currentScreen = "goals";
        renderTabs();
        content.removeAllViews();
        content.addView(sectionTitle("Goals"));
        content.addView(body("Create any do-or-avoid commitment, then act on a selected goal directly."));
        content.addView(goalsToolbar());

        goalList = column();
        addWithMargins(content, goalList, 0, dp(8), 0, 0);
        content.addView(goalList);
        actionPanel = column();
        addWithMargins(content, actionPanel, 0, dp(8), 0, 0);
        content.addView(actionPanel);
        formPanel = column();
        addWithMargins(content, formPanel, 0, dp(8), 0, 0);
        content.addView(formPanel);
        renderGoalList();
        loadGoals();
    }

    private View goalsToolbar() {
        LinearLayout toolbar = row();
        toolbar.addView(button(showCreateForm ? "Close form" : "New goal", showCreateForm ? COLOR_AMBER : COLOR_GREEN, () -> {
            showCreateForm = !showCreateForm;
            showGoals();
        }), new LinearLayout.LayoutParams(0, -2, 1));
        toolbar.addView(button("Refresh", COLOR_BLUE, this::loadGoals), new LinearLayout.LayoutParams(0, -2, 1));
        return toolbar;
    }

    private View createGoalCard() {
        LinearLayout card = card();
        card.addView(cardTitle("New commitment"));
        card.addView(body("MetaMask approval happens in the web app. Android uses the API key from Settings."));

        EditText title = input("Goal title");
        EditText stake = input("Stake amount, e.g. 100");
        stake.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL);
        Spinner type = spinner("do", "avoid");
        Spinner cadence = spinner("daily", "weekly");
        Spinner token = spinner("USDC", "USDT");
        EditText chain = input("Chain key, e.g. sepolia");
        chain.setText("sepolia");
        EditText startsAt = input("Start date, YYYY-MM-DD (optional)");
        EditText endsAt = input("End date, YYYY-MM-DD (optional)");

        card.addView(title);
        card.addView(twoColumns(type, cadence));
        card.addView(stake);
        card.addView(twoColumns(token, chain));
        card.addView(startsAt);
        card.addView(endsAt);
        card.addView(button("Create goal", COLOR_GREEN, () -> run("Creating goal", () -> {
            Goal created = client().createGoal(
                    title.getText().toString(),
                    selected(type),
                    selected(cadence),
                    stake.getText().toString(),
                    selected(token),
                    chain.getText().toString(),
                    startsAt.getText().toString(),
                    endsAt.getText().toString()
            );
            selectedGoalId = created.id;
            return "Created: " + created.title;
        }, result -> {
            showStatus(result, false);
            showCreateForm = false;
            title.setText("");
            stake.setText("");
            loadGoals();
        })));
        return card;
    }

    private void loadGoals() {
        showStatus("Loading goals...", false);
        runQuiet(() -> client().listGoals(), loaded -> {
            if (!"goals".equals(currentScreen)) return;
            goals.clear();
            goals.addAll(loaded);
            if (selectedGoalId.isEmpty() && !goals.isEmpty()) {
                selectedGoalId = goals.get(0).id;
            }
            if (findSelectedGoal() == null && !goals.isEmpty()) {
                selectedGoalId = goals.get(0).id;
                showEditForm = false;
            }
            renderGoalList();
            showStatus(goals.isEmpty() ? "No active goals yet" : goals.size() + " active goal" + (goals.size() == 1 ? "" : "s"), false);
        });
    }

    private void renderGoalList() {
        if (goalList == null) return;
        goalList.removeAllViews();
        LinearLayout header = row();
        TextView title = cardTitle("Active goals");
        header.addView(title, new LinearLayout.LayoutParams(0, -2, 1));
        goalList.addView(header);

        if (goals.isEmpty()) {
            LinearLayout empty = card();
            empty.addView(cardTitle("No active goals"));
            empty.addView(body("Create one below or ask Chat to draft a goal."));
            goalList.addView(empty);
            renderActionPanel();
            renderFormPanel();
            return;
        }

        for (Goal goal : goals) {
            goalList.addView(goalCard(goal));
        }
        renderActionPanel();
        renderFormPanel();
    }

    private View goalCard(Goal goal) {
        LinearLayout card = card();
        boolean selected = goal.id.equals(selectedGoalId);
        card.setBackground(rounded(COLOR_SURFACE, selected ? COLOR_GREEN : COLOR_BORDER, selected ? 2 : 1, 14));
        card.setClickable(true);
        card.setOnClickListener(v -> {
            selectedGoalId = goal.id;
            showEditForm = false;
            renderGoalList();
            showStatus("Selected: " + goal.title, false);
        });

        LinearLayout top = row();
        TextView title = cardTitle(goal.title.isEmpty() ? "Untitled goal" : goal.title);
        top.addView(title, new LinearLayout.LayoutParams(0, -2, 1));
        top.addView(pill(goal.type + " / " + goal.cadence, selected ? COLOR_GREEN : COLOR_BLUE, selected ? Color.rgb(229, 245, 237) : Color.rgb(233, 241, 250)));
        card.addView(top);

        card.addView(body("Stake: " + formatStake(goal.stakeAmount) + " " + goal.tokenSymbol + " on " + goal.chain));
        String schedule = scheduleSummary(goal);
        if (!schedule.isEmpty()) {
            card.addView(caption(schedule));
        }

        TextView hint = caption(selected ? "Selected for actions" : "Tap to select");
        hint.setPadding(0, dp(6), 0, 0);
        card.addView(hint);
        return card;
    }

    private void renderActionPanel() {
        if (actionPanel == null) return;
        actionPanel.removeAllViews();
        Goal selected = findSelectedGoal();
        if (selected == null) {
            return;
        }

        LinearLayout card = card();
        card.addView(cardTitle("Selected goal actions"));
        card.addView(body(selected.title + " is selected. Actions use this goal automatically."));

        LinearLayout quick = row();
        quick.addView(button("Check in", COLOR_GREEN, () -> withSelectedGoal(goal -> run("Recording check-in", () -> {
            client().logCheckIn(goal.id, "done from Android");
            return "Check-in recorded";
        }, result -> showStatus(result, false)))), new LinearLayout.LayoutParams(0, -2, 1));
        quick.addView(button("Violation", COLOR_RED, () -> withSelectedGoal(goal -> run("Reporting violation", () -> {
            client().reportViolation(goal.id, "reported from Android");
            return "Violation reported";
        }, result -> showStatus(result, true)))), new LinearLayout.LayoutParams(0, -2, 1));
        card.addView(quick);

        LinearLayout manage = row();
        manage.addView(button("Show progress", COLOR_BLUE, () -> withSelectedGoal(goal -> run("Loading progress", () -> {
            Progress value = client().getProgress(goal.id);
            return "Completed: " + (value.currentPeriodCompleted ? "yes" : "no") + " | violations: " + value.violationCount;
        }, result -> showStatus(result, false)))), new LinearLayout.LayoutParams(0, -2, 1));
        manage.addView(button(showEditForm ? "Hide edit panel" : "Edit selected goal", COLOR_AMBER, () -> {
            showEditForm = !showEditForm;
            renderGoalList();
        }), new LinearLayout.LayoutParams(0, -2, 1));
        card.addView(manage);

        if (!showEditForm) {
            actionPanel.addView(card);
            return;
        }

        EditText title = input("New title");
        title.setText(selected.title);
        EditText description = input("Description");
        EditText stake = input("Stake amount");
        stake.setText(formatStake(selected.stakeAmount));
        stake.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL);
        EditText endsAt = input("End date, YYYY-MM-DD (blank to keep)");
        EditText chain = input("Chain key");
        chain.setText(selected.chain);
        Spinner token = spinner("USDC", "USDT");
        setSpinnerValue(token, selected.tokenSymbol);

        card.addView(title);
        card.addView(description);
        card.addView(stake);
        card.addView(endsAt);
        card.addView(twoColumns(token, chain));

        card.addView(button("Update title, description, stake, end date", COLOR_BLUE, () -> withSelectedGoal(goal -> run("Updating goal", () -> {
            Goal updated = client().updateGoal(
                    goal.id,
                    title.getText().toString(),
                    description.getText().toString(),
                    stake.getText().toString(),
                    optionalText(endsAt)
            );
            selectedGoalId = updated.id;
            return "Updated: " + updated.title;
        }, result -> {
            showStatus(result, false);
            loadGoals();
        }))));

        card.addView(button("Clear end date", COLOR_BLUE, () -> withSelectedGoal(goal -> run("Clearing end date", () -> {
            Goal updated = client().updateGoal(
                    goal.id,
                    title.getText().toString(),
                    description.getText().toString(),
                    stake.getText().toString(),
                    ""
            );
            selectedGoalId = updated.id;
            return "End date cleared";
        }, result -> {
            showStatus(result, false);
            loadGoals();
        }))));

        card.addView(button("Move stake to selected token/chain", COLOR_AMBER, () -> withSelectedGoal(goal -> run("Moving stake", () -> {
            Goal updated = client().updateStake(goal.id, stake.getText().toString(), selected(token), chain.getText().toString());
            selectedGoalId = updated.id;
            return "Stake moved to " + updated.tokenSymbol + " on " + updated.chain;
        }, result -> {
            showStatus(result, false);
            loadGoals();
        }))));

        card.addView(button("Archive selected goal", COLOR_RED, () -> withSelectedGoal(goal -> run("Archiving goal", () -> {
            client().archiveGoal(goal.id);
            selectedGoalId = "";
            showEditForm = false;
            return "Goal archived";
        }, result -> {
            showStatus(result, false);
            loadGoals();
        }))));

        actionPanel.addView(card);
    }

    private void renderFormPanel() {
        if (formPanel == null) return;
        formPanel.removeAllViews();
        if (showCreateForm || goals.isEmpty()) {
            formPanel.addView(createGoalCard());
        }
    }

    private void showChat() {
        currentScreen = "chat";
        renderTabs();
        content.removeAllViews();
        content.addView(sectionTitle("AI goal manager"));
        content.addView(body("Ask for a goal, report progress, or summarize what is next."));

        LinearLayout card = card();
        chatMessageInput = input("Message, e.g. Create a weekly gym goal with a 25 USDC stake");
        chatMessageInput.setSingleLine(false);
        chatMessageInput.setMinLines(3);
        chatMessageInput.setGravity(Gravity.TOP | Gravity.START);
        chatReplyOutput = body("Replies will appear here.");
        chatReplyOutput.setBackground(rounded(Color.rgb(242, 246, 249), COLOR_BORDER, 1, 12));
        chatReplyOutput.setPadding(dp(12), dp(10), dp(12), dp(10));

        card.addView(chatMessageInput);
        LinearLayout actions = row();
        actions.addView(button("Send", COLOR_GREEN, () -> {
            String value = chatMessageInput.getText().toString().trim();
            if (value.isEmpty()) {
                showStatus("Write a message first", true);
                return;
            }
            sendChatMessage(value);
        }), new LinearLayout.LayoutParams(0, -2, 1));
        actions.addView(button("Voice", COLOR_BLUE, this::startVoiceChat), new LinearLayout.LayoutParams(0, -2, 1));
        card.addView(actions);
        card.addView(chatReplyOutput);
        content.addView(card);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != REQUEST_VOICE_CHAT || resultCode != RESULT_OK || data == null) {
            showStatus("Voice input was canceled", true);
            return;
        }
        ArrayList<String> matches = data.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS);
        if (matches == null || matches.isEmpty() || matches.get(0).trim().isEmpty()) {
            showStatus("Voice input returned no text", true);
            return;
        }
        String transcript = matches.get(0).trim();
        if (chatMessageInput != null) {
            chatMessageInput.setText(transcript);
        }
        sendChatMessage(transcript);
    }

    private void startVoiceChat() {
        Intent intent = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE, Locale.getDefault());
        intent.putExtra(RecognizerIntent.EXTRA_PROMPT, "Goal command");
        try {
            startActivityForResult(intent, REQUEST_VOICE_CHAT);
        } catch (ActivityNotFoundException error) {
            String fallback = chatMessageInput == null ? "" : chatMessageInput.getText().toString().trim();
            if (fallback.isEmpty()) {
                showStatus("Voice input is not available on this device", true);
                return;
            }
            sendChatMessage(fallback);
        }
    }

    private void sendChatMessage(String message) {
        run("Sending message", () -> client().chat(message), result -> {
            if (chatReplyOutput != null) {
                chatReplyOutput.setText(result.isEmpty() ? "No reply returned." : result);
            }
            showStatus("Chat reply received", false);
        });
    }

    private void showSettings() {
        currentScreen = "settings";
        renderTabs();
        content.removeAllViews();
        content.addView(sectionTitle("Settings"));
        content.addView(body("Paste an API key from web Settings. The key is stored locally on this device."));

        SharedPreferences prefs = prefs();
        EditText baseUrlInput = input("API URL");
        baseUrlInput.setText(prefs.getString("base_url", "http://10.0.2.2:8080"));
        EditText apiKeyInput = input("sk_ API key");
        apiKeyInput.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        apiKeyInput.setText(prefs.getString("api_key", ""));

        LinearLayout card = card();
        card.addView(cardTitle("API connection"));
        card.addView(baseUrlInput);
        card.addView(apiKeyInput);
        card.addView(button("Save settings", COLOR_GREEN, () -> {
            prefs.edit()
                    .putString("base_url", baseUrlInput.getText().toString().trim())
                    .putString("api_key", apiKeyInput.getText().toString().trim())
                    .apply();
            showStatus("Settings saved", false);
        }));
        card.addView(button("Test connection", COLOR_BLUE, () -> run("Testing connection", () -> {
            List<Goal> loaded = new ApiClient(baseUrlInput.getText().toString(), apiKeyInput.getText().toString()).listGoals();
            return "Connected. " + loaded.size() + " active goal" + (loaded.size() == 1 ? "" : "s");
        }, result -> showStatus(result, false))));
        content.addView(card);

        LinearLayout agent = card();
        agent.addView(cardTitle("Own agent"));
        agent.addView(body("Generate a private Markdown skill link for your own agent. It contains a generated API secret and daily reminder cron instructions."));
        EditText agentName = input("Agent name");
        agentName.setText("android");
        TextView agentLinkOutput = body("No agent link generated yet.");
        agentLinkOutput.setTextIsSelectable(true);
        final String[] latestAgentLink = {""};
        agent.addView(agentName);
        agent.addView(button("Connect own agent", COLOR_GREEN, () -> run("Creating agent link", () ->
                new ApiClient(baseUrlInput.getText().toString(), apiKeyInput.getText().toString()).createAgentLink(agentName.getText().toString()),
                result -> {
                    latestAgentLink[0] = result;
                    agentLinkOutput.setText(result.isEmpty() ? "No link returned." : result);
                    showStatus("Own-agent link generated", false);
                })));
        agent.addView(button("Copy latest link", COLOR_BLUE, () -> {
            if (latestAgentLink[0].isEmpty()) {
                showStatus("Create an agent link first", true);
                return;
            }
            ClipboardManager clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
            clipboard.setPrimaryClip(ClipData.newPlainText("Goal Stakes agent skill", latestAgentLink[0]));
            showStatus("Agent link copied", false);
        }));
        agent.addView(agentLinkOutput);
        agent.addView(body("Revoke own-agent access from web Settings when the link is no longer needed."));
        content.addView(agent);

        LinearLayout safety = card();
        safety.addView(cardTitle("Money safety"));
        safety.addView(body("Android cannot approve token allowances. Use the web app before increasing stake or changing token/chain."));
        safety.addView(body("Penalties burn from your allowance through the configured StakeEnforcer; the app does not custody funds."));
        content.addView(safety);
    }

    private ApiClient client() {
        SharedPreferences prefs = prefs();
        return new ApiClient(
                prefs.getString("base_url", "http://10.0.2.2:8080"),
                prefs.getString("api_key", "")
        );
    }

    private void run(String pending, Task task, Result result) {
        showStatus(pending + "...", false);
        io.execute(() -> {
            try {
                String value = task.call();
                main.post(() -> result.accept(value));
            } catch (Exception error) {
                main.post(() -> showStatus(readableError(error), true));
            }
        });
    }

    private <T> void runQuiet(ValueTask<T> task, ValueResult<T> result) {
        io.execute(() -> {
            try {
                T value = task.call();
                main.post(() -> result.accept(value));
            } catch (Exception error) {
                main.post(() -> showStatus(readableError(error), true));
            }
        });
    }

    private void showStatus(String message, boolean error) {
        if (statusLine == null) return;
        statusLine.setText(message == null || message.trim().isEmpty() ? "Action failed" : message);
        statusLine.setTextColor(error ? COLOR_RED : COLOR_BLUE);
        statusLine.setBackground(rounded(error ? Color.rgb(253, 235, 233) : Color.rgb(233, 241, 250), error ? COLOR_RED : COLOR_BLUE, 1, 20));
    }

    private Goal findSelectedGoal() {
        for (Goal goal : goals) {
            if (goal.id.equals(selectedGoalId)) {
                return goal;
            }
        }
        return null;
    }

    private void withSelectedGoal(GoalAction action) {
        Goal goal = findSelectedGoal();
        if (goal == null) {
            showStatus("Select a goal first", true);
            return;
        }
        action.run(goal);
    }

    private TextView sectionTitle(String value) {
        TextView view = text(value, compactLayout ? 20 : 24, true);
        view.setPadding(0, compactLayout ? dp(4) : dp(8), 0, dp(2));
        return view;
    }

    private TextView cardTitle(String value) {
        TextView view = text(value, compactLayout ? 16 : 18, true);
        view.setPadding(0, 0, 0, dp(6));
        return view;
    }

    private TextView body(String value) {
        TextView view = text(value, compactLayout ? 13 : 14, false);
        view.setTextColor(COLOR_MUTED);
        view.setLineSpacing(dp(2), 1.0f);
        return view;
    }

    private TextView caption(String value) {
        TextView view = text(value, 12, false);
        view.setTextColor(COLOR_MUTED);
        return view;
    }

    private TextView text(String value, int sp, boolean strong) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(sp);
        view.setTextColor(COLOR_TEXT);
        view.setIncludeFontPadding(true);
        if (strong) view.setTypeface(Typeface.DEFAULT_BOLD);
        return view;
    }

    private TextView pill(String value, int textColor, int bgColor) {
        TextView view = text(value, 12, true);
        view.setTextColor(textColor);
        view.setGravity(Gravity.CENTER);
        view.setSingleLine(false);
        view.setPadding(dp(10), dp(5), dp(10), dp(5));
        view.setBackground(rounded(bgColor, Color.TRANSPARENT, 0, 18));
        return view;
    }

    private EditText input(String hint) {
        EditText edit = new EditText(this);
        edit.setHint(hint);
        edit.setTextColor(COLOR_TEXT);
        edit.setHintTextColor(Color.rgb(117, 127, 140));
        edit.setTextSize(15);
        edit.setSingleLine(true);
        edit.setPadding(dp(12), 0, dp(12), 0);
        edit.setMinHeight(dp(compactLayout ? 42 : 48));
        edit.setBackground(rounded(Color.rgb(250, 251, 250), COLOR_BORDER, 1, 10));
        addDefaultMargins(edit);
        return edit;
    }

    private Spinner spinner(String... values) {
        Spinner spinner = new Spinner(this);
        ArrayAdapter<String> adapter = new ArrayAdapter<>(this, android.R.layout.simple_spinner_item, values);
        adapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);
        spinner.setAdapter(adapter);
        spinner.setPadding(dp(8), 0, dp(8), 0);
        spinner.setMinimumHeight(dp(compactLayout ? 42 : 48));
        spinner.setBackground(rounded(Color.rgb(250, 251, 250), COLOR_BORDER, 1, 10));
        addDefaultMargins(spinner);
        return spinner;
    }

    private Button tab(String label, String screen, Runnable action) {
        boolean active = currentScreen.equals(screen);
        Button button = button(label, active ? COLOR_GREEN : Color.rgb(98, 108, 120), action);
        button.setBackground(rounded(active ? Color.rgb(225, 244, 235) : Color.rgb(232, 235, 237), active ? COLOR_GREEN : COLOR_BORDER, active ? 2 : 1, 12));
        button.setTextColor(active ? COLOR_GREEN : COLOR_TEXT);
        button.setLayoutParams(new LinearLayout.LayoutParams(0, dp(compactLayout ? 38 : 46), 1));
        return button;
    }

    private Button button(String label, int color, Runnable action) {
        Button button = new Button(this);
        button.setText(label);
        button.setTextSize(14);
        button.setAllCaps(false);
        button.setTextColor(Color.WHITE);
        button.setGravity(Gravity.CENTER);
        button.setMinHeight(dp(compactLayout ? 40 : 46));
        button.setPadding(dp(12), 0, dp(12), 0);
        button.setBackground(rounded(color, color, 1, 10));
        button.setOnClickListener(v -> action.run());
        addDefaultMargins(button);
        return button;
    }

    private Button smallButton(String label, int color, Runnable action) {
        Button button = button(label, color, action);
        button.setTextSize(12);
        button.setMinHeight(dp(36));
        button.setPadding(dp(10), 0, dp(10), 0);
        return button;
    }

    private LinearLayout card() {
        LinearLayout card = column();
        card.setPadding(dp(14), dp(14), dp(14), dp(14));
        card.setBackground(rounded(COLOR_SURFACE, COLOR_BORDER, 1, 14));
        card.setElevation(dp(1));
        addWithMargins(contentSafeParent(), card, 0, dp(10), 0, dp(4));
        return card;
    }

    private LinearLayout column() {
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        return layout;
    }

    private LinearLayout row() {
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.HORIZONTAL);
        layout.setGravity(Gravity.CENTER_VERTICAL);
        return layout;
    }

    private LinearLayout twoColumns(View left, View right) {
        LinearLayout row = row();
        row.addView(left, new LinearLayout.LayoutParams(0, -2, 1));
        row.addView(right, new LinearLayout.LayoutParams(0, -2, 1));
        return row;
    }

    private GradientDrawable rounded(int fill, int stroke, int strokeDp, int radiusDp) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(fill);
        drawable.setCornerRadius(dp(radiusDp));
        if (strokeDp > 0) {
            drawable.setStroke(dp(strokeDp), stroke);
        }
        return drawable;
    }

    private LinearLayout contentSafeParent() {
        return content == null ? root : content;
    }

    private void addDefaultMargins(View view) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.setMargins(0, dp(6), 0, dp(6));
        view.setLayoutParams(params);
    }

    private void addWithMargins(LinearLayout parent, View child, int left, int top, int right, int bottom) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.setMargins(left, top, right, bottom);
        child.setLayoutParams(params);
    }

    private SharedPreferences prefs() {
        return getSharedPreferences(PREFS, MODE_PRIVATE);
    }

    private String selected(Spinner spinner) {
        return String.valueOf(spinner.getSelectedItem());
    }

    private void setSpinnerValue(Spinner spinner, String value) {
        for (int i = 0; i < spinner.getCount(); i++) {
            if (String.valueOf(spinner.getItemAtPosition(i)).equalsIgnoreCase(value)) {
                spinner.setSelection(i);
                return;
            }
        }
    }

    private String optionalText(EditText input) {
        String value = input.getText().toString().trim();
        return value.isEmpty() ? null : value;
    }

    private String formatStake(String baseUnits) {
        String raw = baseUnits == null || baseUnits.trim().isEmpty() ? "0" : baseUnits.trim();
        if (!raw.matches("\\d+")) return raw;
        while (raw.length() <= 6) {
            raw = "0" + raw;
        }
        String whole = raw.substring(0, raw.length() - 6).replaceFirst("^0+(?!$)", "");
        String frac = raw.substring(raw.length() - 6).replaceFirst("0+$", "");
        return frac.isEmpty() ? whole : whole + "." + frac;
    }

    private String shortDate(String timestamp) {
        if (timestamp == null) return "";
        return timestamp.length() >= 10 ? timestamp.substring(0, 10) : timestamp;
    }

    private String scheduleSummary(Goal goal) {
        List<String> parts = new ArrayList<>();
        if (!goal.startsAt.isEmpty()) {
            parts.add("starts " + shortDate(goal.startsAt));
        }
        if (!goal.endsAt.isEmpty()) {
            parts.add("ends " + shortDate(goal.endsAt));
        }
        return parts.isEmpty() ? "" : "Schedule: " + join(parts);
    }

    private String join(List<String> values) {
        StringBuilder result = new StringBuilder();
        for (String value : values) {
            if (result.length() > 0) result.append(" | ");
            result.append(value);
        }
        return result.toString();
    }

    private String readableError(Exception error) {
        String value = error.getMessage();
        return value == null || value.trim().isEmpty() ? "Action failed" : value;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private interface Task {
        String call() throws Exception;
    }

    private interface Result {
        void accept(String value);
    }

    private interface ValueTask<T> {
        T call() throws Exception;
    }

    private interface ValueResult<T> {
        void accept(T value);
    }

    private interface GoalAction {
        void run(Goal goal);
    }
}
