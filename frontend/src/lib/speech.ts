type SpeechRecognitionConstructor = new () => SpeechRecognitionLike;

interface SpeechRecognitionEventLike {
  results: ArrayLike<ArrayLike<{ transcript: string }>>;
}

interface SpeechRecognitionLike {
  lang: string;
  interimResults: boolean;
  maxAlternatives: number;
  onresult: ((event: SpeechRecognitionEventLike) => void) | null;
  onerror: ((event: { error?: string }) => void) | null;
  onend: (() => void) | null;
  start: () => void;
  stop?: () => void;
}

export function startVoiceTranscript(onTranscript: (text: string) => void, onEnd: () => void, onError: (message: string) => void) {
  const Recognition = speechRecognitionConstructor();
  if (!Recognition) {
    onError("Voice input is not available in this browser");
    onEnd();
    return null;
  }
  const recognition = new Recognition();
  recognition.lang = navigator.language || "en-US";
  recognition.interimResults = false;
  recognition.maxAlternatives = 1;
  recognition.onresult = (event) => {
    const transcript = event.results?.[0]?.[0]?.transcript?.trim();
    if (transcript) onTranscript(transcript);
  };
  recognition.onerror = (event) => {
    onError(event.error ? `Voice input failed: ${event.error}` : "Voice input failed");
  };
  recognition.onend = onEnd;
  recognition.start();
  return recognition;
}

function speechRecognitionConstructor(): SpeechRecognitionConstructor | undefined {
  const candidate = window as Window & {
    SpeechRecognition?: SpeechRecognitionConstructor;
    webkitSpeechRecognition?: SpeechRecognitionConstructor;
  };
  return candidate.SpeechRecognition ?? candidate.webkitSpeechRecognition;
}
