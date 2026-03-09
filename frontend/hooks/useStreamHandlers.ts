import { useCallback } from "react";
import {
  appendContentDelta as appendContentDeltaToMessages,
  appendThinkingDelta as appendThinkingDeltaToMessages,
  addToolCall as addToolCallToMessages,
  resolveToolCall as resolveToolCallInMessages,
  startThinkingBlock as startThinkingBlockInMessages,
} from "@/lib/chat/streamMutations";
import type { Message } from "@/types/chat";

export interface StreamHandlers {
  appendContentDelta: (runId: string, delta: string, ts: number) => void;
  appendThinkingDelta: (runId: string, delta: string, ts: number) => void;
  startThinkingBlock: (runId: string, ts: number) => void;
  addToolCall: (runId: string, name: string, ts: number, toolCallId?: string, args?: string) => void;
  resolveToolCall: (runId: string, name: string, toolCallId?: string, result?: string, isError?: boolean) => void;
}

export interface UseStreamHandlersOptions {
  /** Called when actual text content arrives (not thinking/tool calls) */
  beginContentArrival: () => void;
  /** Message state setter */
  setMessages: React.Dispatch<React.SetStateAction<Message[]>>;
  /** Streaming ID setter */
  setStreamingId: React.Dispatch<React.SetStateAction<string | null>>;
}

/**
 * Creates stream handler callbacks for processing LLM response chunks.
 * 
 * IMPORTANT: Only `appendContentDelta` calls `beginContentArrival()`.
 * Thinking deltas, tool calls, etc. should NOT trigger content arrival,
 * because we want to calculate thinking duration from message send time
 * to the first actual text content arrival.
 * 
 * BUG SCENARIO: If `appendThinkingDelta` calls `beginContentArrival()`,
 * thinking duration will be calculated from thinking start (e.g., 1s)
 * instead of from text content start (e.g., 30s).
 */
export function useStreamHandlers({
  beginContentArrival,
  setMessages,
  setStreamingId,
}: UseStreamHandlersOptions): StreamHandlers {
  const appendContentDelta = useCallback(
    (runId: string, delta: string, ts: number) => {
      // ✅ Only text content should trigger beginContentArrival
      beginContentArrival();
      setMessages((prev) => {
        const next = appendContentDeltaToMessages(prev, runId, delta, ts);
        if (next.created) setStreamingId(runId);
        return next.messages;
      });
    },
    [beginContentArrival, setMessages, setStreamingId]
  );

  const appendThinkingDelta = useCallback(
    (runId: string, delta: string, ts: number) => {
      // ❌ Don't call beginContentArrival here!
      // Thinking is not "content arrival" - we want duration from
      // thinking start to TEXT content start, not thinking start.
      setMessages((prev) => {
        const next = appendThinkingDeltaToMessages(prev, runId, delta, ts);
        if (next.created) setStreamingId(runId);
        return next.messages;
      });
    },
    [setMessages, setStreamingId]
  );

  const startThinkingBlock = useCallback(
    (runId: string, ts: number) => {
      // ❌ Don't call beginContentArrival here!
      // Thinking block start is not text content arrival.
      setMessages((prev) => {
        const next = startThinkingBlockInMessages(prev, runId, ts);
        if (next.created) setStreamingId(runId);
        return next.messages;
      });
    },
    [setMessages, setStreamingId]
  );

  const addToolCall = useCallback(
    (runId: string, name: string, ts: number, toolCallId?: string, args?: string) => {
      // ❌ Don't call beginContentArrival here!
      // Tool calls are not text content.
      setMessages((prev) => {
        const next = addToolCallToMessages(prev, runId, name, ts, toolCallId, args);
        if (next.created) setStreamingId(runId);
        return next.messages;
      });
    },
    [setMessages, setStreamingId]
  );

  const resolveToolCall = useCallback(
    (runId: string, name: string, toolCallId?: string, result?: string, isError?: boolean) => {
      setMessages((prev) => resolveToolCallInMessages(prev, runId, name, toolCallId, result, isError));
    },
    [setMessages]
  );

  return {
    appendContentDelta,
    appendThinkingDelta,
    startThinkingBlock,
    addToolCall,
    resolveToolCall,
  };
}
