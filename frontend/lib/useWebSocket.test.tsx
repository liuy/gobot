import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

import { useWebSocket } from "@/lib/useWebSocket";

// ── Suppress console.log in tests ────────────────────────────────────────────

const originalLog = console.log;
const originalError = console.error;

beforeEach(() => {
  console.log = vi.fn();
  console.error = vi.fn();
});

afterEach(() => {
  console.log = originalLog;
  console.error = originalError;
});

// ── Mock WebSocket ──────────────────────────────────────────────────────────

type WSListener = (ev: Record<string, unknown>) => void;

class MockWebSocket {
  static instances: MockWebSocket[] = [];

  url: string;
  readyState = 0; // CONNECTING
  onopen: WSListener | null = null;
  onclose: WSListener | null = null;
  onerror: WSListener | null = null;
  onmessage: WSListener | null = null;
  send = vi.fn();
  close = vi.fn(() => {
    this.readyState = 3; // CLOSED
  });

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  /** Simulate the server accepting the TCP/WS connection. */
  simulateOpen() {
    this.readyState = 1; // OPEN
    this.onopen?.({});
  }

  /** Simulate the connection closing (server or network). */
  simulateClose(code = 1006, reason = "") {
    this.readyState = 3;
    this.onclose?.({ code, reason });
  }

  /** Simulate receiving a message. */
  simulateMessage(data: Record<string, unknown>) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  /** Simulate an error event. */
  simulateError() {
    this.onerror?.({});
  }
}

// Replace global WebSocket
const OriginalWebSocket = globalThis.WebSocket;

beforeEach(() => {
  MockWebSocket.instances = [];
  globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;
  vi.useFakeTimers();
});

afterEach(() => {
  globalThis.WebSocket = OriginalWebSocket;
  vi.useRealTimers();
});

// ── Helpers ─────────────────────────────────────────────────────────────────

/** Get the most recently created MockWebSocket instance. */
function lastWS(): MockWebSocket {
  return MockWebSocket.instances[MockWebSocket.instances.length - 1];
}

// ── Tests ───────────────────────────────────────────────────────────────────

describe("useWebSocket — initial retry logic", () => {
  // Note: gobot has INITIAL_RETRY_MAX = 0 (no retry, fail fast)
  // These tests verify the no-retry behavior

  it("fails immediately without retry when connection closes", () => {
    const onInitialRetrying = vi.fn();
    const onInitialConnectFail = vi.fn();

    const { result } = renderHook(() =>
      useWebSocket({ onInitialRetrying, onInitialConnectFail }),
    );

    act(() => { result.current.connect("ws://test"); });
    const ws1 = lastWS();

    act(() => { ws1.simulateClose(); });

    // Should NOT retry, should fail immediately
    expect(onInitialRetrying).not.toHaveBeenCalled();
    expect(onInitialConnectFail).toHaveBeenCalledTimes(1);
    expect(MockWebSocket.instances.length).toBe(1);
  });

  it("does not retry after markEstablished (uses reconnect backoff instead)", () => {
    const onInitialRetrying = vi.fn();
    const onInitialConnectFail = vi.fn();
    const onReconnecting = vi.fn();

    const { result } = renderHook(() =>
      useWebSocket({ onInitialRetrying, onInitialConnectFail, onReconnecting }),
    );

    act(() => { result.current.connect("ws://test"); });
    act(() => { lastWS().simulateOpen(); });

    act(() => { result.current.markEstablished(); });

    act(() => { lastWS().simulateClose(); });

    expect(onInitialRetrying).not.toHaveBeenCalled();
    expect(onInitialConnectFail).not.toHaveBeenCalled();
    expect(onReconnecting).toHaveBeenCalledTimes(1);
  });

  it("connects successfully when server is available", () => {
    const onInitialRetrying = vi.fn();
    const onInitialConnectFail = vi.fn();
    const onOpen = vi.fn();

    const { result } = renderHook(() =>
      useWebSocket({ onInitialRetrying, onInitialConnectFail, onOpen }),
    );

    act(() => { result.current.connect("ws://test"); });
    act(() => { lastWS().simulateOpen(); });

    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(result.current.connectionState).toBe("connected");
    expect(onInitialConnectFail).not.toHaveBeenCalled();
    expect(onInitialRetrying).not.toHaveBeenCalled();
  });
});
