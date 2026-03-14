import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { ChatViewport } from "./ChatViewport";
import type { Message } from "@/types/chat";

// Mock heavy child components
vi.mock("@/components/MessageRow", () => ({
  MessageRow: () => <div data-testid="message-row" />,
}));

vi.mock("@/components/ThinkingIndicator", () => ({
  ThinkingIndicator: () => <div data-testid="thinking-indicator" />,
}));

vi.mock("@/components/ZenToggle", () => ({
  ZenToggle: () => <div data-testid="zen-toggle" />,
}));

// Minimal mock for useSubagentStore
const mockSubagentStore = {
  sessions: {},
  linkingSessionKeys: [],
  registerSpawn: vi.fn(),
  setLinking: vi.fn(),
};

describe("ChatViewport - Running Duration", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  const createMessage = (overrides: Partial<Message> = {}): Message => ({
    id: "msg-1",
    role: "assistant",
    content: "Hello",
    timestamp: Date.now(),
    ...overrides,
  });

  const defaultProps = {
    isDetached: false,
    isNative: false,
    historyLoaded: true,
    inputZoneHeight: "4rem",
    bottomPad: "4rem",
    scrollRef: { current: null },
    bottomRef: { current: null },
    pullContentRef: { current: null },
    pullSpinnerRef: { current: null },
    onScroll: vi.fn(),
    displayMessages: [] as Message[],
    sentAnimId: null,
    onSentAnimationEnd: vi.fn(),
    fadeInIds: new Set<string>(),
    isStreaming: false,
    streamingId: null,
    subagentStore: mockSubagentStore,
    pinnedToolCallId: null,
    onPin: vi.fn(),
    onUnpin: vi.fn(),
    zenMode: false,
    isRunActive: false,
    awaitingResponse: false,
    thinkingLabel: undefined,
    runningDuration: null,
    quotePopup: null,
    quotePopupRef: { current: null },
    onAcceptQuote: vi.fn(),
  };

  it("shows runningDuration for streaming assistant message", () => {
    const messages = [createMessage({ id: "msg-1", role: "assistant" })];

    render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={true}
        streamingId="msg-1"
        runningDuration={5}
      />
    );

    // Should show the hourglass with duration
    expect(screen.getByText("5s")).toBeInTheDocument();
    // Check for the hourglass emoji (grayscale)
    expect(screen.getByText("⌛")).toBeInTheDocument();
  });

  it("updates runningDuration display as time progresses", () => {
    const messages = [createMessage({ id: "msg-1", role: "assistant" })];

    const { rerender } = render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={true}
        streamingId="msg-1"
        runningDuration={5}
      />
    );

    expect(screen.getByText("5s")).toBeInTheDocument();

    // Simulate time passing (parent would update runningDuration)
    rerender(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={true}
        streamingId="msg-1"
        runningDuration={10}
      />
    );

    expect(screen.getByText("10s")).toBeInTheDocument();
  });

  it("shows runDuration from message after streaming ends", () => {
    const messages = [createMessage({ id: "msg-1", role: "assistant", runDuration: 65 })];

    render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={false}
        streamingId={null}
        runningDuration={null}
      />
    );

    // Should show formatted duration (1m 5s)
    expect(screen.getByText("1m 5s")).toBeInTheDocument();
  });

  it("transitions from runningDuration to runDuration without flicker", () => {
    const messages = [createMessage({ id: "msg-1", role: "assistant" })];

    // Streaming with runningDuration
    const { rerender } = render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={true}
        streamingId="msg-1"
        runningDuration={65}
      />
    );

    expect(screen.getByText("1m 5s")).toBeInTheDocument();

    // Stream ends, runDuration is now on message
    const messagesWithRunDuration = [createMessage({ id: "msg-1", role: "assistant", runDuration: 65 })];

    rerender(
      <ChatViewport
        {...defaultProps}
        displayMessages={messagesWithRunDuration}
        isStreaming={false}
        streamingId={null}
        runningDuration={null}
      />
    );

    // Duration should still be visible (from message.runDuration now)
    expect(screen.getByText("1m 5s")).toBeInTheDocument();
  });

  it("does not show duration for user messages", () => {
    const messages = [createMessage({ id: "msg-1", role: "user", runDuration: 10 })];

    render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={false}
        streamingId={null}
      />
    );

    // Duration should not appear for user messages
    expect(screen.queryByText("10s")).not.toBeInTheDocument();
  });

  it("shows longer durations in hours format", () => {
    const messages = [createMessage({ id: "msg-1", role: "assistant", runDuration: 3665 })];

    render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={false}
        streamingId={null}
      />
    );

    // 3665 seconds = 1h 1m 5s → formatted as "1h 1m"
    expect(screen.getByText("1h 1m")).toBeInTheDocument();
  });

  it("does not show duration when runningDuration is null during streaming", () => {
    const messages = [createMessage({ id: "msg-1", role: "assistant" })];

    render(
      <ChatViewport
        {...defaultProps}
        displayMessages={messages}
        isStreaming={true}
        streamingId="msg-1"
        runningDuration={null}
      />
    );

    // No duration displayed yet (thinking hasn't finished)
    expect(screen.queryByText(/s$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/m$/)).not.toBeInTheDocument();
  });
});
