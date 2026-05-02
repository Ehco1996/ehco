import { wsURL } from "./client";

export interface WSHandle {
  close(): void;
}

// Reconnect with capped exponential backoff. Caller's onMessage receives raw
// JSON-decoded log frames; framing errors (non-JSON) are silently dropped so
// a single bad frame doesn't kill the stream.
export function connectLogs(
  onMessage: (frame: unknown) => void,
  onStatus: (s: "open" | "closed" | "error") => void,
): WSHandle {
  let ws: WebSocket | null = null;
  let closed = false;
  let backoff = 500;

  const open = () => {
    if (closed) return;
    ws = new WebSocket(wsURL("/ws/logs"));
    ws.onopen = () => {
      backoff = 500;
      onStatus("open");
    };
    ws.onmessage = (ev) => {
      try {
        onMessage(JSON.parse(ev.data));
      } catch {
        /* ignore framing errors */
      }
    };
    ws.onerror = () => onStatus("error");
    ws.onclose = () => {
      onStatus("closed");
      if (closed) return;
      const wait = Math.min(backoff, 8000);
      backoff = Math.min(backoff * 2, 8000);
      setTimeout(open, wait);
    };
  };

  open();

  return {
    close() {
      closed = true;
      ws?.close();
    },
  };
}
