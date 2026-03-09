"use client";

import { useState, useEffect } from "react";
import { SlideContent } from "@/components/SlideContent";

const THINKING_COLLAPSE_THRESHOLD = 5;
const THINKING_PREVIEW_CHARS = 50; // Last N chars to show when collapsed + streaming

export interface ThinkingPillProps {
  text: string;
  isStreaming?: boolean;
  isThinkingComplete?: boolean;
}

export function ThinkingPill({ text, isStreaming, isThinkingComplete }: ThinkingPillProps) {
  const isEmpty = !text.trim();
  const [mounted, setMounted] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const lineCount = text.split("\n").length;
  const needsClamp = !isEmpty && lineCount >= THINKING_COLLAPSE_THRESHOLD;

  useEffect(() => {
    if (mounted) return;
    const raf = requestAnimationFrame(() => setMounted(true));
    return () => cancelAnimationFrame(raf);
  }, [mounted]);

  if (isEmpty) {
    return (
      <SlideContent open={mounted}>
        <p className="text-xs leading-[1.5] text-muted-foreground/50">
          <span className="inline-flex items-center gap-0.5">
            <span>Thinking</span>
            <span className="inline-flex w-4">
              <span className="animate-[dotFade_1.4s_ease-in-out_infinite]">.</span>
              <span className="animate-[dotFade_1.4s_ease-in-out_0.2s_infinite]">.</span>
              <span className="animate-[dotFade_1.4s_ease-in-out_0.4s_infinite]">.</span>
            </span>
          </span>
        </p>
      </SlideContent>
    );
  }

  if (!needsClamp) {
    return (
      <SlideContent open={mounted}>
        <p className="text-xs leading-[1.5] text-muted-foreground/50 whitespace-pre-wrap break-words overflow-hidden">
          {text}
        </p>
      </SlideContent>
    );
  }

  // Title always shows status
  const getTitle = () => {
    // If thinking is complete (has content after it), show "✓ Thinking finished."
    if (isThinkingComplete || (!isStreaming && !isEmpty)) {
      return "✓ Thinking finished.";
    }
    // Still streaming: show scrolling preview
    if (isStreaming) {
      const lastN = text.slice(-THINKING_PREVIEW_CHARS);
      return text.length > THINKING_PREVIEW_CHARS ? `…${lastN}` : lastN;
    }
    return "✓ Thinking finished.";
  };

  return (
    <SlideContent open={mounted}>
      <div
        role="button"
        tabIndex={0}
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); setExpanded((v) => !v); } }}
        className="text-xs leading-[1.5] text-muted-foreground/50 cursor-pointer"
      >
        {/* Title row - always visible */}
        <div className="flex items-center gap-1">
          <span className="truncate break-words overflow-hidden">{getTitle()}</span>
          <svg
            width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"
            className="shrink-0 opacity-60 transition-transform duration-200"
            style={{ transform: expanded ? "rotate(0deg)" : "rotate(-90deg)" }}
          >
            <path d="m6 9 6 6 6-6" />
          </svg>
        </div>
        {/* Expanded content */}
        <SlideContent open={expanded}>
          <p className="whitespace-pre-wrap break-words overflow-hidden mt-1">{text}</p>
        </SlideContent>
      </div>
    </SlideContent>
  );
}

export { THINKING_COLLAPSE_THRESHOLD, THINKING_PREVIEW_CHARS };
