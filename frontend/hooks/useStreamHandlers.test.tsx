import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { render, screen } from "@testing-library/react";
import { useStreamHandlers } from "./useStreamHandlers";
import { ThinkingPill } from "@/components/ThinkingPill";
import type { Message } from "@/types/chat";

/**
 * Integration test for thinking duration calculation.
 *
 * Tests the full data flow:
 * 1. User sends message → setThinkingStartTime(T0)
 * 2. Thinking delta arrives → appendThinkingDelta() should NOT trigger beginContentArrival()
 * 3. 30 seconds pass...
 * 4. Text content arrives → appendContentDelta() should trigger beginContentArrival()
 * 5. Duration = T_now - T0 = 30s
 * 6. ThinkingPill displays "✓ Thinking finished. (30s)"
 *
 * BUG SCENARIO:
 * If appendThinkingDelta incorrectly calls beginContentArrival(),
 * duration will be calculated at 1s instead of 30s.
 */

describe("useStreamHandlers - Thinking Duration Integration", () => {
  let mockDateNow: ReturnType<typeof vi.spyOn>;
  let currentTime: number;

  beforeEach(() => {
    currentTime = 1000000;
    mockDateNow = vi.spyOn(Date, "now").mockImplementation(() => currentTime);
    vi.spyOn(Storage.prototype, "getItem").mockReturnValue(null);
  });

  afterEach(() => {
    mockDateNow.mockRestore();
    vi.restoreAllMocks();
  });

  describe("beginContentArrival call behavior", () => {
    it("appendContentDelta should call beginContentArrival", () => {
      const beginContentArrival = vi.fn();
      const setMessages = vi.fn();
      const setStreamingId = vi.fn();

      const { result } = renderHook(() =>
        useStreamHandlers({
          beginContentArrival,
          setMessages,
          setStreamingId,
        })
      );

      act(() => {
        result.current.appendContentDelta("run-1", "Hello", currentTime);
      });

      expect(beginContentArrival).toHaveBeenCalledTimes(1);
    });

    it("appendThinkingDelta should NOT call beginContentArrival", () => {
      const beginContentArrival = vi.fn();
      const setMessages = vi.fn();
      const setStreamingId = vi.fn();

      const { result } = renderHook(() =>
        useStreamHandlers({
          beginContentArrival,
          setMessages,
          setStreamingId,
        })
      );

      act(() => {
        result.current.appendThinkingDelta("run-1", "Thinking...", currentTime);
      });

      expect(beginContentArrival).not.toHaveBeenCalled();
    });

    it("startThinkingBlock should NOT call beginContentArrival", () => {
      const beginContentArrival = vi.fn();
      const setMessages = vi.fn();
      const setStreamingId = vi.fn();

      const { result } = renderHook(() =>
        useStreamHandlers({
          beginContentArrival,
          setMessages,
          setStreamingId,
        })
      );

      act(() => {
        result.current.startThinkingBlock("run-1", currentTime);
      });

      expect(beginContentArrival).not.toHaveBeenCalled();
    });

    it("addToolCall should NOT call beginContentArrival", () => {
      const beginContentArrival = vi.fn();
      const setMessages = vi.fn();
      const setStreamingId = vi.fn();

      const { result } = renderHook(() =>
        useStreamHandlers({
          beginContentArrival,
          setMessages,
          setStreamingId,
        })
      );

      act(() => {
        result.current.addToolCall("run-1", "search", currentTime, "tool-1", "{}");
      });

      expect(beginContentArrival).not.toHaveBeenCalled();
    });
  });

  describe("full integration: thinking → content → UI", () => {
    it("calculates correct duration when thinking delta arrives before content", () => {
      // Setup: track when beginContentArrival is called
      let beginContentArrivalCallTime: number | null = null;
      let thinkingDuration: number | null = null;

      const beginContentArrival = vi.fn(() => {
        beginContentArrivalCallTime = currentTime;
      });

      // Setup: track pendingThinkingDuration from useThinkingState
      const pendingDurationRef = { current: null as number | null };

      const setMessages = vi.fn();
      const setStreamingId = vi.fn();

      const { result } = renderHook(() =>
        useStreamHandlers({
          beginContentArrival,
          setMessages,
          setStreamingId,
        })
      );

      // 1. User sends message, thinking starts at T0
      const thinkingStartTime = currentTime;

      // 2. Thinking delta arrives after 1 second
      currentTime += 1000;
      mockDateNow.mockImplementation(() => currentTime);

      act(() => {
        result.current.appendThinkingDelta("run-1", "Let me think...", currentTime);
      });

      // beginContentArrival should NOT have been called
      expect(beginContentArrivalCallTime).toBeNull();

      // 3. More thinking... 29 more seconds pass (total 30s)
      currentTime += 29000;
      mockDateNow.mockImplementation(() => currentTime);

      act(() => {
        result.current.appendThinkingDelta("run-1", " Still thinking...", currentTime);
      });

      // Still should NOT have been called
      expect(beginContentArrivalCallTime).toBeNull();

      // 4. Text content finally arrives
      act(() => {
        result.current.appendContentDelta("run-1", "Here's my answer!", currentTime);
      });

      // NOW beginContentArrival should have been called
      expect(beginContentArrivalCallTime).toBe(currentTime);

      // 5. Calculate duration (simulating what useThinkingState does)
      thinkingDuration = Math.round((beginContentArrivalCallTime - thinkingStartTime) / 1000);
      expect(thinkingDuration).toBe(30);

      // 6. Verify ThinkingPill displays correct duration
      const longText = Array(6).fill("Thinking content here.").join("\n");
      render(
        <ThinkingPill
          text={longText}
          isStreaming={false}
          isThinkingComplete={true}
          duration={thinkingDuration!}
        />
      );

      expect(screen.getByText("✓ Thinking finished. (30s)")).toBeInTheDocument();
    });

    it("ensures bug scenario would show wrong duration (1s instead of 30s)", () => {
      // This test documents what would happen with the bug:
      // If appendThinkingDelta called beginContentArrival at 1s,
      // duration would be 1s instead of 30s.

      const thinkingStartTime = currentTime;

      // Bug scenario: beginContentArrival called at 1s (for thinking delta)
      let buggyCallTime = thinkingStartTime + 1000;

      // Calculate buggy duration
      const buggyDuration = Math.round((buggyCallTime - thinkingStartTime) / 1000);
      expect(buggyDuration).toBe(1); // ❌ Wrong! Should be 30s

      // Correct scenario: beginContentArrival called at 30s (for text content)
      const correctCallTime = thinkingStartTime + 30000;

      // Calculate correct duration
      const correctDuration = Math.round((correctCallTime - thinkingStartTime) / 1000);
      expect(correctDuration).toBe(30); // ✅ Correct!

      // Verify UI would show different values
      const longText = Array(6).fill("Thinking content.").join("\n");

      const { rerender } = render(
        <ThinkingPill text={longText} isThinkingComplete={true} duration={buggyDuration} />
      );
      expect(screen.getByText("✓ Thinking finished. (1s)")).toBeInTheDocument(); // Bug

      rerender(
        <ThinkingPill text={longText} isThinkingComplete={true} duration={correctDuration} />
      );
      expect(screen.getByText("✓ Thinking finished. (30s)")).toBeInTheDocument(); // Fixed
    });
  });
});
