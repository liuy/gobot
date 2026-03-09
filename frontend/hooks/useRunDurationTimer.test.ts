import { renderHook, act } from "@testing-library/react";
import { useRunDurationTimer } from "./useRunDurationTimer";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";

describe("useRunDurationTimer", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("starts with null duration", () => {
    const { result } = renderHook(() => useRunDurationTimer());
    expect(result.current.runningDuration).toBeNull();
  });

  it("startTimer sets initial duration and increments every second", () => {
    const { result } = renderHook(() => useRunDurationTimer());

    act(() => {
      result.current.startTimer();
    });

    // Initial duration should be 0 or 1 (depending on timing)
    expect(result.current.runningDuration).toBeGreaterThanOrEqual(0);

    // After 1 second
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.runningDuration).toBe(1);

    // After another second
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.runningDuration).toBe(2);
  });

  it("startTimer with startTime calculates correct initial duration", () => {
    const { result } = renderHook(() => useRunDurationTimer());
    const fiveSecondsAgo = Date.now() - 5000;

    act(() => {
      result.current.startTimer(fiveSecondsAgo);
    });

    expect(result.current.runningDuration).toBe(5);
  });

  it("stopTimer stops the timer and returns final duration", () => {
    const { result } = renderHook(() => useRunDurationTimer());

    act(() => {
      result.current.startTimer();
    });

    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(result.current.runningDuration).toBe(3);

    let finalDuration: number | null = null;
    act(() => {
      finalDuration = result.current.stopTimer();
    });

    expect(finalDuration).toBe(3);

    // Timer should not increment after stop
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current.runningDuration).toBe(3);
  });

  it("resetTimer clears duration and stops timer", () => {
    const { result } = renderHook(() => useRunDurationTimer());

    act(() => {
      result.current.startTimer();
    });

    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current.runningDuration).toBe(2);

    act(() => {
      result.current.resetTimer();
    });

    expect(result.current.runningDuration).toBeNull();

    // Timer should not increment after reset
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current.runningDuration).toBeNull();
  });

  it("calling startTimer again restarts the timer", () => {
    const { result } = renderHook(() => useRunDurationTimer());

    act(() => {
      result.current.startTimer();
    });

    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(result.current.runningDuration).toBe(3);

    // Restart with new start time
    act(() => {
      result.current.startTimer();
    });

    expect(result.current.runningDuration).toBe(0);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.runningDuration).toBe(1);
  });

  it("cleans up interval on unmount", () => {
    const { result, unmount } = renderHook(() => useRunDurationTimer());

    act(() => {
      result.current.startTimer();
    });

    // Should have an interval running
    expect(vi.getTimerCount()).toBe(1);

    unmount();

    // Interval should be cleared after unmount
    expect(vi.getTimerCount()).toBe(0);
  });
});
