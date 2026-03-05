"use client";

import { useState, useRef, useEffect } from "react";
import type { ConnectionConfig } from "@/types/chat";

// In dev mode, use relative path (proxied by Next.js). In prod, use same host.
function getDefaultServerUrl(): string {
  if (typeof window === "undefined") return "ws://127.0.0.1:10086";
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws`;
}

export function SetupDialog({
  onConnect,
  onClose,
  visible,
  connectionError,
}: {
  onConnect: (config: ConnectionConfig) => void;
  onClose?: () => void;
  visible: boolean;
  connectionError?: string | null;
}) {
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [phase, setPhase] = useState<"idle" | "entering" | "open" | "closing" | "closed">("idle");
  const phaseRef = useRef(phase);
  phaseRef.current = phase;
  const inputRef = useRef<HTMLInputElement>(null);

  // Reset phase when dialog becomes visible again
  useEffect(() => {
    const currentPhase = phaseRef.current;
    if (visible && (currentPhase === "closed" || currentPhase === "idle")) {
      setIsSubmitting(false);
      // Pre-fill token from localStorage if available
      const savedToken = window.localStorage.getItem("openclaw-token");
      if (savedToken) setToken(savedToken);
      setError("");
      requestAnimationFrame(() => {
        setPhase("entering");
        requestAnimationFrame(() => setPhase("open"));
      });
    }
    if (!visible && currentPhase === "open") {
      setPhase("closing");
      setTimeout(() => setPhase("closed"), 500);
    }
  }, [visible]);

  // Reset submitting state on connection error
  useEffect(() => {
    if (connectionError) setIsSubmitting(false);
  }, [connectionError]);

  // Focus input once open
  useEffect(() => {
    if (phase === "open") {
      setTimeout(() => inputRef.current?.focus(), 300);
    }
  }, [phase]);

  const handleSubmit = () => {
    setError("");
    setIsSubmitting(true);
    setPhase("closing");
    setTimeout(() => {
      setPhase("closed");
      onConnect({
        mode: "openclaw",
        url: getDefaultServerUrl(),
        token: token.trim() || undefined,
      });
    }, 500);
  };

  if (phase === "closed" || (!visible && phase === "idle")) return null;

  const isOpen = phase === "open";
  const isClosing = phase === "closing";

  return (
    <div
      className="absolute inset-0 z-50 flex items-center justify-center transition-all duration-500 ease-out"
      style={{
        backdropFilter: isOpen ? "blur(8px)" : "blur(0px)",
        opacity: isClosing ? 0 : isOpen ? 1 : 0,
        pointerEvents: isClosing ? "none" : "auto",
      }}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-background/80 transition-opacity duration-500"
        style={{ opacity: isOpen ? 1 : 0 }}
        onClick={onClose ? () => { setPhase("closing"); setTimeout(() => { setPhase("closed"); onClose(); }, 500); } : undefined}
      />

      {/* Card */}
      <div
        className="relative mx-4 w-full max-w-sm rounded-2xl border border-border bg-card p-6 shadow-lg transition-all duration-500 ease-out"
        style={{
          transform: isOpen
            ? "scale(1) translateY(0)"
            : isClosing
              ? "scale(0.8) translateY(-40px)"
              : "scale(0.9) translateY(20px)",
          opacity: isOpen ? 1 : isClosing ? 0 : 0,
        }}
      >
        {/* Icon */}
        <div className="mb-4 flex justify-center">
          <div className="flex h-12 w-12 items-center justify-center rounded-full border border-border bg-secondary">
            <img src="/logo.png" alt="" className="h-9 mix-blend-multiply dark:mix-blend-screen dark:invert" />
          </div>
        </div>

        <h2 className="mb-1 text-center text-lg font-semibold text-foreground">Connect to GoBot</h2>
        <p className="mb-4 text-center text-sm text-muted-foreground">
          Enter your gateway token to connect.
        </p>

        {/* Token input */}
        <div className="mb-4">
          <label htmlFor="gateway-token" className="mb-1.5 block text-xs font-medium text-muted-foreground">
            Gateway Token
          </label>
          <input
            ref={inputRef}
            id="gateway-token"
            type="password"
            value={token}
            onChange={(e) => { setToken(e.target.value); setError(""); }}
            onKeyDown={(e) => { if (e.key === "Enter") handleSubmit(); }}
            placeholder="Enter gateway auth token"
            disabled={isSubmitting}
            className={`w-full rounded-xl border bg-background px-4 py-2.5 font-mono text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-50 ${error || connectionError ? "border-destructive" : "border-border"}`}
          />
        </div>

        {error && <p className="mb-3 text-xs text-destructive">{error}</p>}
        {connectionError && <p className="mb-3 text-xs text-destructive">{connectionError}</p>}

        {/* Connect button */}
        <button
          type="button"
          onClick={handleSubmit}
          disabled={isSubmitting}
          className="w-full rounded-xl bg-primary py-2.5 text-sm font-medium text-primary-foreground transition-colors hover:opacity-90 disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {isSubmitting ? (
            <>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="animate-spin">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
              Connecting...
            </>
          ) : (
            "Connect"
          )}
        </button>

        <p className="mt-3 text-center text-xs text-muted-foreground/60">
          Server: {getDefaultServerUrl()}
        </p>
      </div>
    </div>
  );
}
