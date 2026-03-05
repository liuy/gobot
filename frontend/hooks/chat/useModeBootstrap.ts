import { useEffect, useCallback, useRef } from "react";

import { notifyWebViewReady, registerBridgeHandler, updateBridgeHandler, type BridgeMessage } from "@/lib/nativeBridge";
import type { Command } from "@/components/CommandSheet";
import type { BackendMode, ConnectionConfig, Message } from "@/types/chat";

/** Read a URL search param. Returns null when absent or during SSR. */
function getSearchParam(name: string): string | null {
  if (typeof window === "undefined") return null;
  return new URLSearchParams(window.location.search).get(name);
}

/** Convert an HTTP(S) URL to its WS(S) equivalent. Already-WS URLs pass through. */
function toWsUrl(url: string): string {
  if (url.startsWith("ws://") || url.startsWith("wss://")) return url;
  return url.replace(/^http:\/\//, "ws://").replace(/^https:\/\//, "wss://");
}

interface UseModeBootstrapOptions {
  connect: (url: string) => void;
  disconnect: () => void;
  handleNativeBridgeMessage: (msg: BridgeMessage) => void;
  resetThinkingState: () => void;
  gatewayTokenRef: React.MutableRefObject<string | null>;
  setOpenclawUrl: React.Dispatch<React.SetStateAction<string | null>>;
  setMessages: React.Dispatch<React.SetStateAction<Message[]>>;
  setConnectionError: React.Dispatch<React.SetStateAction<string | null>>;
  setBackendMode: React.Dispatch<React.SetStateAction<BackendMode>>;
  setShowSetup: React.Dispatch<React.SetStateAction<boolean>>;
  setHistoryLoaded: React.Dispatch<React.SetStateAction<boolean>>;
  setIsInitialConnecting: React.Dispatch<React.SetStateAction<boolean>>;
  setServerCommands: React.Dispatch<React.SetStateAction<Command[]>>;
  isDetachedRef: React.MutableRefObject<boolean>;
  isNativeRef: React.MutableRefObject<boolean>;
}

export function useModeBootstrap({
  connect,
  disconnect,
  handleNativeBridgeMessage,
  resetThinkingState,
  gatewayTokenRef,
  setOpenclawUrl,
  setMessages,
  setConnectionError,
  setBackendMode,
  setShowSetup,
  setHistoryLoaded,
  setIsInitialConnecting,
  setServerCommands,
  isDetachedRef,
  isNativeRef,
}: UseModeBootstrapOptions) {
  const hasInitializedRef = useRef(false);

  useEffect(() => {
    if (hasInitializedRef.current) return;
    hasInitializedRef.current = true;

    if (isNativeRef.current) {
      registerBridgeHandler((msg: BridgeMessage) => {
        handleNativeBridgeMessage(msg);
      });
      notifyWebViewReady();
    }
  }, [
    handleNativeBridgeMessage,
    isNativeRef,
  ]);

  // Keep the bridge handler fresh — the bootstrap effect only registers once,
  // but handleNativeBridgeMessage may be recreated when its deps change.
  useEffect(() => {
    if (isNativeRef.current) {
      updateBridgeHandler((msg: BridgeMessage) => {
        handleNativeBridgeMessage(msg);
      });
    }
  }, [handleNativeBridgeMessage, isNativeRef]);

  useEffect(() => {
    // In native mode, web waits for config:connection from Swift — no auto-connect.
    if (isNativeRef.current) return;

    const detached = isDetachedRef.current;
    const embedUrl = getSearchParam("url");
    if (detached && embedUrl) {
      gatewayTokenRef.current = getSearchParam("token");
      setBackendMode("openclaw");
      setOpenclawUrl(embedUrl);
      setIsInitialConnecting(true);
      connect(toWsUrl(embedUrl));
      return;
    }

    const savedMode = window.localStorage.getItem("gobot-mode");
    // Clear old demo mode setting and show setup dialog
    if (savedMode === "demo") {
      window.localStorage.removeItem("gobot-mode");
      if (!detached) setShowSetup(true);
      setHistoryLoaded(true);
      return;
    }

    // OpenClaw mode (default)
    const savedUrl = window.localStorage.getItem("openclaw-url");
    const savedToken = window.localStorage.getItem("openclaw-token");
    if (savedUrl) {
      gatewayTokenRef.current = savedToken ?? null;
      setBackendMode("openclaw");
      setOpenclawUrl(savedUrl);
      try {
        const cached = localStorage.getItem("mc-server-commands");
        if (cached) {
          setServerCommands(JSON.parse(cached) as Command[]);
        }
      } catch {}
      setIsInitialConnecting(true);
      connect(toWsUrl(savedUrl));
    } else {
      if (!detached) setShowSetup(true);
      setHistoryLoaded(true);
    }
  }, [
    connect,
    isDetachedRef,
    isNativeRef,
    gatewayTokenRef,
    setBackendMode,
    setHistoryLoaded,
    setIsInitialConnecting,
    setMessages,
    setOpenclawUrl,
    setServerCommands,
    setShowSetup,
  ]);

  const handleConnect = useCallback((config: ConnectionConfig) => {
    setConnectionError(null);
    setMessages([]);
    resetThinkingState();
    window.localStorage.removeItem("lmstudio-messages");

    // OpenClaw mode
    window.localStorage.setItem("gobot-mode", "openclaw");
    if (config.remember) {
      window.localStorage.setItem("openclaw-url", config.url);
      if (config.token) window.localStorage.setItem("openclaw-token", config.token);
      else window.localStorage.removeItem("openclaw-token");
    } else {
      window.localStorage.removeItem("openclaw-url");
      window.localStorage.removeItem("openclaw-token");
    }
    gatewayTokenRef.current = config.token ?? null;
    setBackendMode("openclaw");
    setOpenclawUrl(config.url);
    setIsInitialConnecting(true);
    connect(toWsUrl(config.url));
  }, [
    connect,
    disconnect,
    gatewayTokenRef,
    resetThinkingState,
    setBackendMode,
    setConnectionError,
    setHistoryLoaded,
    setIsInitialConnecting,
    setMessages,
    setOpenclawUrl,
  ]);

  return {
    handleConnect,
    initCompleteFlags: {
      hasMounted: true,
    },
  };
}
