// Gateway Browser Client for GoBot
// Handles WebSocket connection with OpenClaw Gateway protocol handshake

import {
  loadOrCreateDeviceIdentity,
  signDevicePayload,
  buildDeviceAuthPayload,
  type DeviceIdentity,
} from "../deviceIdentity";

const GATEWAY_CLIENT_ID = "openclaw-control-ui";
const GATEWAY_CLIENT_MODE = "webchat";

const CONNECT_FAILED_CLOSE_CODE = 4008;

// Types matching GoBot's WSIncomingMessage
type WSResponse = {
  type: "res";
  id: string;
  ok: boolean;
  payload?: unknown;
  error?: string | { code: string; message: string };
};

type WSEvent = {
  type: "event";
  event: string;
  payload?: unknown;
  seq?: number;
  stateVersion?: { presence: number; health: number };
};

type WSHello = {
  type: "hello";
  sessionId: string;
};

type WSIncomingMessage = WSResponse | WSEvent | WSHello;

export type GatewayHelloOk = {
  type: "hello-ok";
  protocol: number;
  features?: { methods?: string[]; events?: string[] };
  snapshot?: unknown;
  auth?: {
    deviceToken?: string;
    role?: string;
    scopes?: string[];
    issuedAtMs?: number;
  };
  policy?: { tickIntervalMs?: number };
};

type Pending = {
  resolve: (value: unknown) => void;
  reject: (err: unknown) => void;
};

export type GatewayClientOptions = {
  url: string;
  token?: string;
  authScopeKey?: string;
  disableDeviceAuth?: boolean;
  onHello?: (hello: GatewayHelloOk) => void;
  onEvent?: (evt: WSIncomingMessage) => void;
  onClose?: (info: { code: number; reason: string }) => void;
  onError?: (error: Error) => void;
};

export class GatewayBrowserClient {
  private ws: WebSocket | null = null;
  private pending = new Map<string, Pending>();
  private closed = false;
  private lastSeq: number | null = null;
  private connectNonce: string | null = null;
  private connectSent = false;
  private connectTimer: number | null = null;
  private backoffMs = 800;
  private deviceIdentity: DeviceIdentity | null = null;

  constructor(private opts: GatewayClientOptions) {}

  start() {
    this.closed = false;
    this.connect();
  }

  stop() {
    this.closed = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.flushPending(new Error("gateway client stopped"));
  }

  get connected() {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private connect() {
    if (this.closed) return;

    console.log("[GatewayClient] Connecting to", this.opts.url);
    this.ws = new WebSocket(this.opts.url);

    this.ws.onopen = () => {
      console.log("[GatewayClient] WebSocket opened, queueing connect");
      this.queueConnect();
    };

    this.ws.onmessage = (ev) => this.handleMessage(String(ev.data ?? ""));

    this.ws.onclose = (ev) => {
      const reason = String(ev.reason ?? "");
      console.log("[GatewayClient] WebSocket closed:", ev.code, reason);
      this.ws = null;
      this.flushPending(new Error(`gateway closed (${ev.code}): ${reason}`));
      this.opts.onClose?.({ code: ev.code, reason });
      this.scheduleReconnect();
    };

    this.ws.onerror = (error) => {
      console.error("[GatewayClient] WebSocket error:", error);
      console.error("[GatewayClient] Error type:", error.type);
      console.error("[GatewayClient] URL being connected:", this.opts.url);
      this.opts.onError?.(new Error("WebSocket error"));
    };
  }

  private scheduleReconnect() {
    if (this.closed) return;
    const delay = this.backoffMs;
    this.backoffMs = Math.min(this.backoffMs * 1.7, 15000);
    console.log(`[GatewayClient] Scheduling reconnect in ${delay}ms`);
    window.setTimeout(() => this.connect(), delay);
  }

  private flushPending(err: Error) {
    for (const [, p] of this.pending) p.reject(err);
    this.pending.clear();
  }

  private async sendConnect() {
    if (this.connectSent) return;
    this.connectSent = true;

    if (this.connectTimer !== null) {
      window.clearTimeout(this.connectTimer);
      this.connectTimer = null;
    }

    const isSecureContext =
      !this.opts.disableDeviceAuth &&
      typeof crypto !== "undefined" &&
      !!crypto.subtle;

    const scopes = ["operator.read", "operator.write", "operator.admin", "operator.approvals", "operator.pairing"];
    const role = "operator";
    // Note: authScopeKey would be used for device token storage if implemented
    // const _authScopeKey = this.normalizeAuthScope(this.opts.authScopeKey ?? this.opts.url);

    let deviceIdentity = this.deviceIdentity;

    // Load or create device identity
    if (isSecureContext && !deviceIdentity) {
      try {
        deviceIdentity = await loadOrCreateDeviceIdentity();
        this.deviceIdentity = deviceIdentity;
      } catch (err) {
        console.warn("[GatewayClient] Failed to load device identity:", err);
      }
    }

    const authToken = this.opts.token;

    const auth =
      authToken
        ? { token: authToken }
        : undefined;

    let device:
      | {
          id: string;
          publicKey: string;
          signature: string;
          signedAt: number;
          nonce: string | undefined;
        }
      | undefined;

    if (isSecureContext && deviceIdentity) {
      const signedAtMs = Date.now();
      const nonce = this.connectNonce ?? undefined;
      const payload = buildDeviceAuthPayload({
        deviceId: deviceIdentity.deviceId,
        clientId: GATEWAY_CLIENT_ID,
        clientMode: GATEWAY_CLIENT_MODE,
        role,
        scopes,
        signedAtMs,
        token: authToken ?? null,
        nonce,
      });
      const signature = await signDevicePayload(deviceIdentity.privateKey, payload);
      device = {
        id: deviceIdentity.deviceId,
        publicKey: deviceIdentity.publicKey,
        signature,
        signedAt: signedAtMs,
        nonce,
      };
    }

    const params = {
      minProtocol: 3,
      maxProtocol: 3,
      client: {
        id: GATEWAY_CLIENT_ID,
        version: "1.0.0",
        platform: navigator.platform ?? "web",
        mode: GATEWAY_CLIENT_MODE,
      },
      role,
      scopes,
      device,
      caps: ["tool-events"],
      auth,
      userAgent: navigator.userAgent,
      locale: navigator.language,
    };

    console.log("[GatewayClient] Sending connect request with params:", JSON.stringify(params, null, 2));

    this.request<GatewayHelloOk>("connect", params)
      .then((hello) => {
        console.log("[GatewayClient] Connect successful:", hello);
        this.backoffMs = 800;
        this.opts.onHello?.(hello);
      })
      .catch((err) => {
        console.error("[GatewayClient] Connect failed:", err);
        const rawReason = err instanceof Error ? `connect failed: ${err.message}` : "connect failed";
        const reason = this.truncateWsCloseReason(rawReason);
        this.ws?.close(CONNECT_FAILED_CLOSE_CODE, reason);
      });
  }

  // Note: normalizeAuthScope would be used for device token storage if implemented
  // private normalizeAuthScope(scope: string): string {
  //   const trimmed = scope?.trim();
  //   if (!trimmed) return "default";
  //   return trimmed.toLowerCase();
  // }

  private truncateWsCloseReason(reason: string, maxBytes = 123): string {
    const trimmed = reason.trim();
    if (!trimmed) return "connect failed";
    const encoder = new TextEncoder();
    if (encoder.encode(trimmed).byteLength <= maxBytes) return trimmed;

    let out = "";
    for (const char of trimmed) {
      const next = out + char;
      if (encoder.encode(next).byteLength > maxBytes) break;
      out = next;
    }
    return out.trimEnd() || "connect failed";
  }

  private handleMessage(raw: string) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch {
      console.warn("[GatewayClient] Failed to parse message:", raw.slice(0, 100));
      return;
    }

    const frame = parsed as { type?: unknown };

    if (frame.type === "event") {
      const evt = parsed as WSEvent;

      // Handle connect.challenge - respond with auth
      if (evt.event === "connect.challenge") {
        const payload = evt.payload as { nonce?: unknown } | undefined;
        const nonce = payload && typeof payload.nonce === "string" ? payload.nonce : null;
        console.log("[GatewayClient] Received connect.challenge, nonce:", nonce);
        if (nonce) {
          this.connectNonce = nonce;
          void this.sendConnect();
        }
        return;
      }

      // Handle sequence numbers for gap detection
      const seq = typeof evt.seq === "number" ? evt.seq : null;
      if (seq !== null) {
        if (this.lastSeq !== null && seq > this.lastSeq + 1) {
          console.warn("[GatewayClient] Gap detected:", this.lastSeq + 1, "->", seq);
        }
        this.lastSeq = seq;
      }

      try {
        this.opts.onEvent?.(evt);
      } catch (err) {
        console.error("[GatewayClient] Event handler error:", err);
      }
      return;
    }

    if (frame.type === "res") {
      const res = parsed as WSResponse;

      // Also dispatch to onEvent for compatibility with existing message handling
      const evt: WSIncomingMessage = {
        type: "res",
        id: res.id,
        ok: res.ok,
        payload: res.payload,
        error: res.error,
      };
      try {
        this.opts.onEvent?.(evt);
      } catch (err) {
        console.error("[GatewayClient] Response event handler error:", err);
      }

      const pending = this.pending.get(res.id);
      if (!pending) {
        console.log("[GatewayClient] Ignoring response for unknown id:", res.id);
        return;
      }
      this.pending.delete(res.id);
      if (res.ok) {
        pending.resolve(res.payload);
      } else {
        // Handle error - can be string or { code, message }
        if (res.error) {
          if (typeof res.error === "object" && "code" in res.error) {
            const errObj = res.error as { code: string; message?: string };
            pending.reject(
              new Error(`${errObj.code}: ${errObj.message ?? "request failed"}`)
            );
          } else {
            pending.reject(new Error(String(res.error)));
          }
          return;
        }
        pending.reject(new Error("request failed"));
      }
      return;
    }
  }

  request<T = unknown>(method: string, params?: unknown): Promise<T> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error("gateway not connected"));
    }

    const id = `req-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const frame = { type: "req", id, method, params };

    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, {
        resolve: (v) => resolve(v as T),
        reject,
      });
      this.ws?.send(JSON.stringify(frame));
    });
  }

  private queueConnect() {
    this.connectNonce = null;
    this.connectSent = false;
    if (this.connectTimer !== null) window.clearTimeout(this.connectTimer);
    // Delay to ensure WebSocket is ready (match studio's 750ms)
    this.connectTimer = window.setTimeout(() => {
      void this.sendConnect();
    }, 750);
  }
}
