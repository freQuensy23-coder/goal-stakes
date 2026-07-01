import { afterEach, describe, expect, it } from "vitest";
import { startVoiceTranscript } from "../../src/lib/speech";

const originalWindow = globalThis.window;
const originalNavigator = globalThis.navigator;

afterEach(() => {
  Object.defineProperty(globalThis, "window", { value: originalWindow, configurable: true });
  Object.defineProperty(globalThis, "navigator", { value: originalNavigator, configurable: true });
});

function installBrowserGlobals(windowValue: Record<string, unknown>, language = "en-US") {
  Object.defineProperty(globalThis, "window", { value: windowValue, configurable: true });
  Object.defineProperty(globalThis, "navigator", { value: { language }, configurable: true });
}

describe("startVoiceTranscript", () => {
  it("reports a friendly error when speech recognition is unavailable", () => {
    installBrowserGlobals({});
    const errors: string[] = [];
    let ended = false;

    const recognition = startVoiceTranscript(
      () => {
        throw new Error("transcript should not be called");
      },
      () => {
        ended = true;
      },
      (message) => errors.push(message),
    );

    expect(recognition).toBeNull();
    expect(errors).toEqual(["Voice input is not available in this browser"]);
    expect(ended).toBe(true);
  });

  it("starts recognition and emits trimmed transcript text", () => {
    let created: FakeRecognition | undefined;
    class FakeRecognition {
      lang = "";
      interimResults = true;
      maxAlternatives = 0;
      onresult: ((event: { results: ArrayLike<ArrayLike<{ transcript: string }>> }) => void) | null = null;
      onerror: ((event: { error?: string }) => void) | null = null;
      onend: (() => void) | null = null;
      started = false;

      constructor() {
        created = this;
      }

      start() {
        this.started = true;
      }
    }
    installBrowserGlobals({ webkitSpeechRecognition: FakeRecognition }, "fr-FR");
    const transcripts: string[] = [];
    const errors: string[] = [];
    let ended = false;

    const recognition = startVoiceTranscript(
      (text) => transcripts.push(text),
      () => {
        ended = true;
      },
      (message) => errors.push(message),
    );

    expect(recognition).toBe(created);
    expect(created?.started).toBe(true);
    expect(created?.lang).toBe("fr-FR");
    expect(created?.interimResults).toBe(false);
    expect(created?.maxAlternatives).toBe(1);

    created?.onresult?.({ results: [[{ transcript: "  Create a daily goal  " }]] });
    created?.onerror?.({ error: "not-allowed" });
    created?.onend?.();

    expect(transcripts).toEqual(["Create a daily goal"]);
    expect(errors).toEqual(["Voice input failed: not-allowed"]);
    expect(ended).toBe(true);
  });
});
