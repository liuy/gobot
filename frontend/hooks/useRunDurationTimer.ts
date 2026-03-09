import { useState, useRef, useCallback, useEffect } from "react";

/**
 * Manages a running duration timer that updates every second.
 * Started when thinking ends, stopped when lifecycle:end is received.
 */
export function useRunDurationTimer() {
  const [runningDuration, setRunningDuration] = useState<number | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const startTimeRef = useRef<number>(0);

  const clearTimer = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  /** Start the timer from a given start time (defaults to now) */
  const startTimer = useCallback((startTime?: number) => {
    clearTimer();
    startTimeRef.current = startTime ?? Date.now();
    const initialDuration = Math.round((Date.now() - startTimeRef.current) / 1000);
    setRunningDuration(initialDuration);

    intervalRef.current = setInterval(() => {
      const duration = Math.round((Date.now() - startTimeRef.current) / 1000);
      setRunningDuration(duration);
    }, 1000);
  }, [clearTimer]);

  /** Stop the timer and return final duration */
  const stopTimer = useCallback((): number | null => {
    clearTimer();
    const final = runningDuration;
    return final;
  }, [clearTimer, runningDuration]);

  /** Reset timer state */
  const resetTimer = useCallback(() => {
    clearTimer();
    setRunningDuration(null);
    startTimeRef.current = 0;
  }, [clearTimer]);

  // Cleanup on unmount
  useEffect(() => {
    return () => clearTimer();
  }, [clearTimer]);

  return {
    runningDuration,
    startTimer,
    stopTimer,
    resetTimer,
  };
}
