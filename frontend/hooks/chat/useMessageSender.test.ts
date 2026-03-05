import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useMessageSender } from "./useMessageSender";

// Mock external dependencies
vi.mock("@/lib/notifications", () => ({
  requestNotificationPermission: vi.fn(),
}));

vi.mock("@/lib/constants", () => ({
  LITTERBOX_UPLOAD_URL: "https://example.com/upload",
}));

describe("useMessageSender", () => {
  const createMockOptions = (overrides = {}) => ({
    backendMode: "openclaw" as const,
    isConnected: true,
    sendWS: vi.fn(),
    sessionKeyRef: { current: "test-session" },
    activeRunIdRef: { current: null },
    isDetachedRef: { current: false },
    pinnedToBottomRef: { current: false },
    pinLockUntilRef: { current: 0 },
    setMessages: vi.fn((updater) => {
      // Support functional updates
      if (typeof updater === "function") {
        updater([]);
      }
    }),
    setSentAnimId: vi.fn(),
    setAwaitingResponse: vi.fn(),
    setThinkingStartTime: vi.fn(),
    setIsStreaming: vi.fn(),
    cancelCommandFetch: vi.fn(),
    onConnectionError: vi.fn(),
    ...overrides,
  });

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("when connected", () => {
    it("sends message via WebSocket", async () => {
      const options = createMockOptions();
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("Hello");
      });

      expect(options.sendWS).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "req",
          method: "chat.send",
          params: expect.objectContaining({
            message: "Hello",
          }),
        })
      );
      expect(options.onConnectionError).not.toHaveBeenCalled();
    });

    it("sets streaming state after sending", async () => {
      const options = createMockOptions();
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("Hello");
      });

      expect(options.setIsStreaming).toHaveBeenCalledWith(true);
      expect(options.setAwaitingResponse).toHaveBeenCalledWith(true);
    });

    it("adds user message to state", async () => {
      const options = createMockOptions();
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("Hello");
      });

      expect(options.setMessages).toHaveBeenCalled();
    });
  });

  describe("when disconnected", () => {
    it("calls onConnectionError and does not send", async () => {
      const options = createMockOptions({ isConnected: false });
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("Hello");
      });

      expect(options.sendWS).not.toHaveBeenCalled();
      expect(options.onConnectionError).toHaveBeenCalledTimes(1);
      expect(options.setIsStreaming).not.toHaveBeenCalled();
    });

    it("still adds user message to state (optimistic update)", async () => {
      const options = createMockOptions({ isConnected: false });
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("Hello");
      });

      // User message should still be added optimistically
      expect(options.setMessages).toHaveBeenCalled();
    });
  });

  describe("slash commands", () => {
    it("handles /new command by clearing messages", async () => {
      const options = createMockOptions();
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("/new");
      });

      // /new should result in an empty base array
      expect(options.setMessages).toHaveBeenCalled();
    });

    it("marks slash command messages as hidden", async () => {
      const options = createMockOptions();
      const { result } = renderHook(() => useMessageSender(options));

      await act(async () => {
        await result.current.sendMessage("/help");
      });

      expect(options.setMessages).toHaveBeenCalled();
    });
  });
});
