#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
ADB="${ANDROID_HOME}/platform-tools/adb"
EMULATOR="${ANDROID_HOME}/emulator/emulator"
AVD_NAME="${ANDROID_AVD_NAME:-twinby_mitm}"
EVIDENCE_DIR="$ROOT/.e2e/android-emulator"
PACKAGE="com.goalstakes.mobile"
API_PORT="${GOALSTAKES_ANDROID_API_PORT:-18080}"
API_BASE="http://10.0.2.2:${API_PORT}"
HOST_API_BASE="http://127.0.0.1:${API_PORT}"
TAB_Y="${ANDROID_TAB_Y:-568}"

mkdir -p "$EVIDENCE_DIR"
rm -f "$EVIDENCE_DIR"/*.png "$EVIDENCE_DIR"/window-*.xml

started_emulator=0
emulator_pid=""
api_pid=""
cleanup() {
  if [[ -n "$api_pid" ]]; then
    kill "$api_pid" >/dev/null 2>&1 || true
    wait "$api_pid" >/dev/null 2>&1 || true
  fi
  if [[ "$started_emulator" == "1" ]]; then
    "$ADB" emu kill >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

has_adb_device() {
  "$ADB" devices | awk 'NR > 1 && $2 == "device" { found = 1 } END { exit(found ? 0 : 1) }'
}

wait_for_adb_device() {
  for _ in {1..300}; do
    if has_adb_device; then
      return 0
    fi
    if [[ -n "$emulator_pid" ]] && ! kill -0 "$emulator_pid" >/dev/null 2>&1; then
      echo "Android emulator process exited before adb reported a device" >&2
      tail -n 120 "$EVIDENCE_DIR/emulator.log" >&2 || true
      exit 1
    fi
    sleep 1
  done

  echo "Android emulator did not appear in adb devices" >&2
  "$ADB" devices >&2 || true
  tail -n 120 "$EVIDENCE_DIR/emulator.log" >&2 || true
  exit 1
}

echo "starting Android smoke API on host port $API_PORT"
GOALSTAKES_ANDROID_API_PORT="$API_PORT" node "$ROOT/android-app/integration_test/fake_api.mjs" \
  >"$EVIDENCE_DIR/fake-api.log" 2>&1 &
api_pid=$!
for i in {1..50}; do
  if curl -fsS "$HOST_API_BASE/api/v1/goals" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$api_pid" >/dev/null 2>&1; then
    echo "Android smoke API exited before becoming ready" >&2
    cat "$EVIDENCE_DIR/fake-api.log" >&2 || true
    exit 1
  fi
  if [[ "$i" == "50" ]]; then
    echo "Android smoke API did not become ready" >&2
    cat "$EVIDENCE_DIR/fake-api.log" >&2 || true
    exit 1
  fi
  sleep 0.2
done

if ! has_adb_device; then
  echo "starting Android emulator $AVD_NAME"
  nohup "$EMULATOR" -avd "$AVD_NAME" -no-window -no-audio -no-boot-anim -gpu swiftshader_indirect \
    >"$EVIDENCE_DIR/emulator.log" 2>&1 &
  emulator_pid=$!
  started_emulator=1
fi

echo "waiting for emulator boot"
wait_for_adb_device
for i in {1..120}; do
  if "$ADB" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r' | grep -q '^1$'; then
    break
  fi
  if [[ "$i" == "120" ]]; then
    echo "emulator did not boot" >&2
    tail -n 120 "$EVIDENCE_DIR/emulator.log" >&2 || true
    exit 1
  fi
  sleep 2
done

"$ADB" shell input keyevent 82 >/dev/null 2>&1 || true

echo "installing and launching app"
(cd "$ROOT/android-app" && ANDROID_HOME="$ANDROID_HOME" gradle installDebug >/tmp/goalstakes-android-install.log)
cp /tmp/goalstakes-android-install.log "$EVIDENCE_DIR/install.log"

"$ADB" shell pm clear "$PACKAGE" >/dev/null 2>&1 || true
prefs_xml="<?xml version='1.0' encoding='utf-8' standalone='yes' ?><map><string name=\"base_url\">${API_BASE}</string><string name=\"api_key\">sk_android_smoke</string></map>"
printf '%s\n' "$prefs_xml" | "$ADB" exec-in run-as "$PACKAGE" sh -c 'mkdir -p shared_prefs && cat > shared_prefs/goalstakes.xml'

"$ADB" shell logcat -c >/dev/null 2>&1 || true
"$ADB" shell am start -n "$PACKAGE/.MainActivity" >"$EVIDENCE_DIR/launch.log" 2>&1
app_started=0
for _ in {1..15}; do
  if "$ADB" shell pidof "$PACKAGE" >/dev/null 2>&1; then
    app_started=1
    break
  fi
  sleep 1
done
if [[ "$app_started" != "1" ]]; then
  "$ADB" shell logcat -d -t 300 >"$EVIDENCE_DIR/logcat-launch.log" 2>/dev/null || true
  echo "Goal Stakes app is not running after launch" >&2
  cat "$EVIDENCE_DIR/launch.log" >&2
  tail -n 120 "$EVIDENCE_DIR/logcat-launch.log" >&2 || true
  exit 1
fi
dump_window() {
  local name="$1"
  "$ADB" shell uiautomator dump /sdcard/goalstakes-window.xml >/dev/null 2>&1
  "$ADB" exec-out cat /sdcard/goalstakes-window.xml > "$EVIDENCE_DIR/$name"
  if grep -q "isn't responding" "$EVIDENCE_DIR/$name" && grep -q 'text="Wait"' "$EVIDENCE_DIR/$name"; then
    tap_window_node "$name" "Wait" || true
    sleep 2
    "$ADB" shell uiautomator dump /sdcard/goalstakes-window.xml >/dev/null 2>&1
    "$ADB" exec-out cat /sdcard/goalstakes-window.xml > "$EVIDENCE_DIR/$name"
  fi
}

assert_window_text() {
  local file="$1"
  local text="$2"
  if ! grep -q "$text" "$EVIDENCE_DIR/$file"; then
    echo "Expected '$text' in Android UI dump $file" >&2
    tail -n 80 "$EVIDENCE_DIR/$file" >&2 || true
    "$ADB" shell logcat -d -t 120 >&2 2>/dev/null || true
    exit 1
  fi
}

assert_window_any_text() {
  local file="$1"
  shift
  for text in "$@"; do
    if grep -q "$text" "$EVIDENCE_DIR/$file"; then
      return 0
    fi
  done
  echo "Expected one of '$*' in Android UI dump $file" >&2
  tail -n 80 "$EVIDENCE_DIR/$file" >&2 || true
  "$ADB" shell logcat -d -t 120 >&2 2>/dev/null || true
  exit 1
}

tap_tab() {
  local x="$1"
  "$ADB" shell input tap "$x" "$TAB_Y" >/dev/null 2>&1 || true
  sleep 1
}

tap_tab_text() {
  local text="$1"
  dump_window "window-action.xml"
  tap_window_node "window-action.xml" "$text" "android.widget.Button"
  sleep 1
}

tap_window_node() {
  local file="$1"
  local text="$2"
  local class_filter="${3:-}"
  local coords
  coords="$(node - "$EVIDENCE_DIR/$file" "$text" "$class_filter" <<'NODE'
const fs = require("node:fs");
const file = process.argv[2];
const target = process.argv[3];
const classFilter = process.argv[4] || "";
const xml = fs.readFileSync(file, "utf8");
const tags = xml.match(/<node\b[^>]*>/g) || [];
function attr(tag, name) {
  const match = tag.match(new RegExp(`${name}="([^"]*)"`));
  return match ? match[1] : "";
}
for (const tag of tags) {
  if (classFilter && attr(tag, "class") !== classFilter) continue;
  const text = attr(tag, "text");
  const desc = attr(tag, "content-desc");
  if (text !== target && desc !== target) continue;
  const bounds = attr(tag, "bounds");
  const match = bounds.match(/\[(\d+),(\d+)\]\[(\d+),(\d+)\]/);
  if (!match) continue;
  const left = Number(match[1]);
  const top = Number(match[2]);
  const right = Number(match[3]);
  const bottom = Number(match[4]);
  console.log(`${Math.round((left + right) / 2)} ${Math.round((top + bottom) / 2)}`);
  process.exit(0);
}
console.error(`Could not find ${classFilter || "node"} with text/content-desc "${target}" in ${file}`);
process.exit(1);
NODE
)"
  "$ADB" shell input tap $coords >/dev/null 2>&1
  sleep 0.5
}

wait_for_window_text() {
  local file="$1"
  local text="$2"
  local attempts="${3:-20}"
  for _ in $(seq 1 "$attempts"); do
    dump_window "$file"
    if grep -q "$text" "$EVIDENCE_DIR/$file"; then
      return 0
    fi
    sleep 0.5
  done
  assert_window_text "$file" "$text"
}

tap_text() {
  local text="$1"
  dump_window "window-action.xml"
  tap_window_node "window-action.xml" "$text"
}

tap_edit_text() {
  local text="$1"
  dump_window "window-action.xml"
  tap_window_node "window-action.xml" "$text" "android.widget.EditText"
}

adb_input_text() {
  local value="${1// /%s}"
  "$ADB" shell input text "$value" >/dev/null 2>&1
  sleep 0.4
}

clear_focused_text() {
  "$ADB" shell input keyevent 123 >/dev/null 2>&1 || true
  for _ in {1..80}; do
    "$ADB" shell input keyevent 67 >/dev/null 2>&1 || true
  done
}

hide_keyboard() {
  if "$ADB" shell dumpsys input_method 2>/dev/null | tr -d '\r' | grep -Eq "mInputShown=true|mIsInputViewShown=true"; then
    "$ADB" shell input keyevent 4 >/dev/null 2>&1 || true
  else
    "$ADB" shell input keyevent 111 >/dev/null 2>&1 || true
  fi
  sleep 0.5
}

swipe_later() {
  local count="$1"
  for _ in $(seq 1 "$count"); do
    "$ADB" shell input swipe 540 2100 540 760 500 >/dev/null 2>&1 || true
    sleep 0.5
  done
}

swipe_earlier() {
  local count="$1"
  for _ in $(seq 1 "$count"); do
    "$ADB" shell input swipe 540 760 540 2100 500 >/dev/null 2>&1 || true
    sleep 0.5
  done
}

swipe_to_text() {
  local text="$1"
  local direction="${2:-later}"
  for i in {1..8}; do
    dump_window "window-find.xml"
    local state
    state="$(node - "$EVIDENCE_DIR/window-find.xml" "$text" <<'NODE'
const fs = require("node:fs");
const [file, target] = process.argv.slice(2);
const xml = fs.readFileSync(file, "utf8");
const tags = xml.match(/<node\b[^>]*>/g) || [];
function attr(tag, name) {
  const match = tag.match(new RegExp(`${name}="([^"]*)"`));
  return match ? match[1] : "";
}
for (const tag of tags) {
  const text = attr(tag, "text");
  const desc = attr(tag, "content-desc");
  if (text !== target && desc !== target) continue;
  const match = attr(tag, "bounds").match(/\[(\d+),(\d+)\]\[(\d+),(\d+)\]/);
  if (!match) continue;
  const top = Number(match[2]);
  const bottom = Number(match[4]);
  const center = Math.round((top + bottom) / 2);
  if (center < 760) {
    console.log("above");
  } else if (center > 2150 || bottom - top < 40) {
    console.log("below");
  } else {
    console.log("ok");
  }
  process.exit(0);
}
console.log("missing");
NODE
)"
    if [[ "$state" == "ok" ]]; then
      return 0
    fi
    if [[ "$state" == "above" ]]; then
      "$ADB" shell input swipe 540 760 540 2100 500 >/dev/null 2>&1 || true
    elif [[ "$state" == "below" ]]; then
      "$ADB" shell input swipe 540 2100 540 760 500 >/dev/null 2>&1 || true
    elif [[ "$direction" == "earlier" ]]; then
      "$ADB" shell input swipe 540 760 540 2100 500 >/dev/null 2>&1 || true
    else
      "$ADB" shell input swipe 540 2100 540 760 500 >/dev/null 2>&1 || true
    fi
    sleep 0.5
  done
  echo "Could not reveal '$text' in Android UI" >&2
  exit 1
}

assert_api_goal() {
  local id="$1"
  local title="$2"
  local stake="${3:-}"
  local file="$EVIDENCE_DIR/ui-flow-goals.json"
  for _ in {1..20}; do
    curl -fsS "$HOST_API_BASE/api/v1/goals" > "$file"
    if node - "$file" "$id" "$title" "$stake" <<'NODE'
const fs = require("node:fs");
const [file, id, title, stake] = process.argv.slice(2);
const goals = JSON.parse(fs.readFileSync(file, "utf8"));
const goal = goals.find((item) => item.id === id);
if (!goal) throw new Error(`missing goal ${id}`);
if (goal.title !== title) throw new Error(`goal ${id} title ${goal.title}, want ${title}`);
if (stake && goal.stake_amount !== stake) throw new Error(`goal ${id} stake ${goal.stake_amount}, want ${stake}`);
NODE
    then
      return 0
    fi
    sleep 0.3
  done
  curl -fsS "$HOST_API_BASE/api/v1/goals" > "$file"
  node - "$file" "$id" "$title" "$stake" <<'NODE'
const fs = require("node:fs");
const [file, id, title, stake] = process.argv.slice(2);
const goals = JSON.parse(fs.readFileSync(file, "utf8"));
const goal = goals.find((item) => item.id === id);
if (!goal) throw new Error(`missing goal ${id}`);
if (goal.title !== title) throw new Error(`goal ${id} title ${goal.title}, want ${title}`);
if (stake && goal.stake_amount !== stake) throw new Error(`goal ${id} stake ${goal.stake_amount}, want ${stake}`);
NODE
}

assert_api_goal_missing() {
  local id="$1"
  local file="$EVIDENCE_DIR/ui-flow-goals.json"
  for _ in {1..20}; do
    curl -fsS "$HOST_API_BASE/api/v1/goals" > "$file"
    if node - "$file" "$id" <<'NODE'
const fs = require("node:fs");
const [file, id] = process.argv.slice(2);
const goals = JSON.parse(fs.readFileSync(file, "utf8"));
if (goals.some((item) => item.id === id)) throw new Error(`goal ${id} should have been archived`);
NODE
    then
      return 0
    fi
    sleep 0.3
  done
  curl -fsS "$HOST_API_BASE/api/v1/goals" > "$file"
  node - "$file" "$id" <<'NODE'
const fs = require("node:fs");
const [file, id] = process.argv.slice(2);
const goals = JSON.parse(fs.readFileSync(file, "utf8"));
if (goals.some((item) => item.id === id)) throw new Error(`goal ${id} should have been archived`);
NODE
}

ui_ready=0
for _ in {1..30}; do
  dump_window "window-launch-ready.xml"
  if grep -q "Goal Stakes" "$EVIDENCE_DIR/window-launch-ready.xml"; then
    ui_ready=1
    break
  fi
  sleep 0.5
done
if [[ "$ui_ready" != "1" ]]; then
  echo "Goal Stakes UI did not become visible after launch" >&2
  tail -n 80 "$EVIDENCE_DIR/window-launch-ready.xml" >&2 || true
  exit 1
fi
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/portrait.png"

dump_window "window-initial.xml"
if grep -q "Failed to connect" "$EVIDENCE_DIR/window-initial.xml"; then
  echo "Android app rendered a connection error against the smoke API" >&2
  exit 1
fi

found_goal=0
for i in {1..8}; do
  "$ADB" shell uiautomator dump /sdcard/goalstakes-window.xml >/dev/null 2>&1
  "$ADB" exec-out cat /sdcard/goalstakes-window.xml > "$EVIDENCE_DIR/window-scroll-${i}.xml"
  if grep -q "Android smoke goal" "$EVIDENCE_DIR/window-scroll-${i}.xml"; then
    cp "$EVIDENCE_DIR/window-scroll-${i}.xml" "$EVIDENCE_DIR/window-goal-visible.xml"
    found_goal=1
    break
  fi
  "$ADB" shell input swipe 540 2100 540 700 500 >/dev/null 2>&1 || true
  sleep 0.5
done
if [[ "$found_goal" != "1" ]]; then
  echo "Android app did not expose the smoke goal in the emulator UI" >&2
  tail -n 40 "$EVIDENCE_DIR"/window-scroll-*.xml >&2 || true
  exit 1
fi
swipe_to_text "Edit selected goal"
tap_text "Edit selected goal"
swipe_to_text "Update title, description, stake, end date"
dump_window "window-goals-scrolled.xml"
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/goals-scrolled.png"

tap_tab_text "Chat"
wait_for_window_text "window-chat.xml" "AI goal manager"
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/chat.png"
tap_text "Voice"
sleep 1
"$ADB" shell input keyevent 4 >/dev/null 2>&1 || true
sleep 1
dump_window "window-chat-voice.xml"
assert_window_any_text "window-chat-voice.xml" "Voice input is not available on this device" "Voice input was canceled" "Voice input returned no text"
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/chat-voice.png"

tap_tab_text "Settings"
wait_for_window_text "window-settings.xml" "API connection"
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/settings.png"
swipe_to_text "Connect own agent"
tap_text "Connect own agent"
sleep 1
wait_for_window_text "window-settings-agent.xml" "agt_android.md"
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/settings-agent.png"
swipe_to_text "$API_BASE" "earlier"
tap_edit_text "$API_BASE"
clear_focused_text
adb_input_text "not_a_url"
hide_keyboard
tap_text "Test connection"
sleep 0.8
wait_for_window_text "window-settings-invalid-url.xml" "API URL must start with http:// or https://"
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/settings-invalid-url.png"

tap_tab_text "Goals"

echo "exercising Android visible goal actions"
sleep 1
swipe_to_text "Android smoke goal"
tap_edit_text "Android smoke goal"
clear_focused_text
adb_input_text "Android UI updated"
hide_keyboard
dump_window "window-ui-flow-title-edited.xml"
assert_window_text "window-ui-flow-title-edited.xml" "Android UI updated"
swipe_to_text "Update title, description, stake, end date"
sleep 0.5
tap_text "Update title, description, stake, end date"
sleep 1.2
assert_api_goal "android-fake-goal" "Android UI updated" "1000000"

tap_tab_text "Goals"
sleep 1
swipe_to_text "Show progress"
tap_text "Show progress"
sleep 0.7
dump_window "window-ui-flow-progress.xml"
assert_window_text "window-ui-flow-progress.xml" "Completed: yes | violations: 0"

tap_tab_text "Goals"
sleep 1
swipe_to_text "Archive selected goal"
tap_text "Archive selected goal"
sleep 1.2
dump_window "window-ui-flow-archive.xml"
assert_api_goal_missing "android-fake-goal"

tap_tab_text "Goals"
sleep 1
swipe_to_text "Goal title"
tap_edit_text "Goal title"
adb_input_text "Android UI created"
hide_keyboard
swipe_to_text "Stake amount, e.g. 100"
tap_edit_text "Stake amount, e.g. 100"
adb_input_text "2.5"
hide_keyboard
"$ADB" shell input keyevent 4 >/dev/null 2>&1 || true
sleep 0.8
swipe_to_text "Create goal"
tap_text "Create goal"
sleep 1.2
assert_api_goal "android-created-1" "Android UI created" "2500000"
"$ADB" shell am force-stop "$PACKAGE" >/dev/null 2>&1 || true
"$ADB" shell am start -n "$PACKAGE/.MainActivity" >/dev/null 2>&1
sleep 2
wait_for_window_text "window-ui-flow-created.xml" "Android UI created"

echo "capturing landscape screenshot"
"$ADB" shell settings put system accelerometer_rotation 0 >/dev/null 2>&1 || true
"$ADB" shell settings put system user_rotation 1 >/dev/null 2>&1 || true
sleep 2
"$ADB" exec-out screencap -p > "$EVIDENCE_DIR/landscape.png"
"$ADB" shell settings put system user_rotation 0 >/dev/null 2>&1 || true

file "$EVIDENCE_DIR/portrait.png" "$EVIDENCE_DIR/goals-scrolled.png" "$EVIDENCE_DIR/chat.png" "$EVIDENCE_DIR/chat-voice.png" "$EVIDENCE_DIR/settings.png" "$EVIDENCE_DIR/settings-agent.png" "$EVIDENCE_DIR/settings-invalid-url.png" "$EVIDENCE_DIR/landscape.png"
echo "android emulator e2e passed"
