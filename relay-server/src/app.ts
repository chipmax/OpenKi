import { createHash, timingSafeEqual } from "node:crypto";

/**
 * Phiên bản relay. Giữ khớp với "version" trong package.json.
 */
export const VERSION = "1.0.0";
export const DEFAULT_MAX_BODY_BYTES = 64 * 1024 * 1024;
export const DEFAULT_TIMEOUT_MS = 300_000;
export const DEFAULT_REGION = "local";
export const DEFAULT_PORT = 8789;

const RELAY_TOKEN_HEADER = "x-loimo-relay-token";
const RELAY_UPSTREAM_HEADER = "x-loimo-relay-upstream";

// Hop-by-hop headers must never be forwarded (RFC 9110 §7.6.1). `host` is
// dropped because fetch derives it from the target URL.
const HOP_BY_HOP: Record<string, true> = {
  connection: true,
  "keep-alive": true,
  "proxy-authenticate": true,
  "proxy-authorization": true,
  "proxy-connection": true,
  te: true,
  trailer: true,
  trailers: true,
  "transfer-encoding": true,
  upgrade: true,
  host: true,
};

const NO_BODY_STATUS: Record<number, true> = { 101: true, 204: true, 205: true, 304: true };

export interface RelayConfig {
  readonly token: string;
  readonly allowedHosts: ReadonlySet<string>;
  readonly allowHttp: boolean;
  readonly region: string;
  readonly version: string;
  readonly maxBodyBytes: number;
  readonly timeoutMs: number;
}

export type RelayHandler = (request: Request) => Promise<Response>;

export function loadConfig(env: Record<string, string | undefined> = process.env): RelayConfig {
  const allowedHosts = new Set<string>();
  for (const entry of (env.LOIMO_RELAY_HOSTS ?? "").split(",")) {
    const host = normalizeAllowedHost(entry);
    if (host) allowedHosts.add(host);
  }

  return {
    token: firstNonEmpty(env.LOIMO_RELAY_TOKEN) ?? "",
    allowedHosts,
    allowHttp: (env.LOIMO_RELAY_ALLOW_HTTP ?? "").trim() === "1",
    region: firstNonEmpty(env.LOIMO_RELAY_REGION, env.VERCEL_REGION) ?? DEFAULT_REGION,
    version: VERSION,
    maxBodyBytes: positiveInt(env.LOIMO_RELAY_MAX_BODY_BYTES, DEFAULT_MAX_BODY_BYTES),
    timeoutMs: positiveInt(env.LOIMO_RELAY_TIMEOUT_MS, DEFAULT_TIMEOUT_MS),
  };
}

export function createRelayHandler(config: RelayConfig): RelayHandler {
  return async function handleRelayRequest(request: Request): Promise<Response> {
    const startedAt = Date.now();
    const pathname = pathnameOf(request.url);
    let response: Response;

    try {
      response = await dispatch(request, config);
    } catch {
      // Không log token, header hay body.
      response = errorResponse(500, "internal_error", "relay failed to process the request");
    }

    response.headers.set("x-relay-region", config.region);
    const egress = clientEgress(request.headers);
    if (egress) response.headers.set("x-relay-egress", egress);

    console.log(
      JSON.stringify({
        level: response.status >= 500 ? "error" : "info",
        msg: "relay_request",
        method: request.method,
        path: pathname,
        status: response.status,
        ms: Date.now() - startedAt,
      }),
    );

    return response;
  };
}

async function dispatch(request: Request, config: RelayConfig): Promise<Response> {
  let requestUrl: URL;
  try {
    requestUrl = new URL(request.url);
  } catch {
    return errorResponse(400, "invalid_request_url", "request URL is not absolute");
  }

  if (requestUrl.pathname === "/healthz") {
    return healthResponse(request, config);
  }

  // Fail closed: thiếu token cấu hình thì không phục vụ proxy.
  if (!config.token) {
    return errorResponse(503, "relay_not_configured", "LOIMO_RELAY_TOKEN is not set");
  }

  const presentedToken = request.headers.get(RELAY_TOKEN_HEADER) ?? "";
  if (!constantTimeEquals(presentedToken, config.token)) {
    return errorResponse(401, "unauthorized", "missing or invalid X-LoiMo-Relay-Token");
  }

  const rawUpstream = (request.headers.get(RELAY_UPSTREAM_HEADER) ?? "").trim();
  if (!rawUpstream) {
    return errorResponse(400, "missing_upstream", "X-LoiMo-Relay-Upstream is required");
  }

  let upstream: URL;
  try {
    upstream = new URL(rawUpstream);
  } catch {
    return errorResponse(400, "invalid_upstream", "X-LoiMo-Relay-Upstream must be an absolute scheme://host URL");
  }

  if (upstream.username || upstream.password) {
    return errorResponse(403, "upstream_not_allowed", "credentials in X-LoiMo-Relay-Upstream are not allowed");
  }

  if (upstream.pathname !== "/" || upstream.search || upstream.hash) {
    return errorResponse(400, "invalid_upstream", "X-LoiMo-Relay-Upstream must contain only scheme and host");
  }

  const isHttps = upstream.protocol === "https:";
  const isAllowedHttp = config.allowHttp && upstream.protocol === "http:";
  if (!isHttps && !isAllowedHttp) {
    return errorResponse(
      403,
      "insecure_upstream",
      config.allowHttp
        ? "upstream must use http or https"
        : "upstream must use https; set LOIMO_RELAY_ALLOW_HTTP=1 only for local testing",
    );
  }

  const host = upstream.host.toLowerCase().replace(/\.$/, "");
  const hostname = upstream.hostname.toLowerCase().replace(/\.$/, "");
  // Danh sách rỗng từ chối tất cả (fail closed), không có proxy mở.
  if (!config.allowedHosts.has(host) && !config.allowedHosts.has(hostname)) {
    return errorResponse(403, "upstream_not_allowed", "upstream host is not in LOIMO_RELAY_HOSTS");
  }

  // Giữ nguyên path + query của request gốc, chỉ thay origin.
  const target = new URL(request.url);
  target.protocol = upstream.protocol;
  target.host = upstream.host;
  target.hash = "";

  const hasRequestBody = request.method !== "GET" && request.method !== "HEAD";
  let body: ArrayBuffer | undefined;

  if (hasRequestBody) {
    const declaredLength = Number(request.headers.get("content-length") ?? "");
    if (Number.isFinite(declaredLength) && declaredLength > config.maxBodyBytes) {
      return errorResponse(413, "request_too_large", `request body exceeds ${config.maxBodyBytes} bytes`);
    }

    try {
      const read = await readBody(request, config.maxBodyBytes);
      if (read === TOO_LARGE) {
        return errorResponse(413, "request_too_large", `request body exceeds ${config.maxBodyBytes} bytes`);
      }
      if (read.byteLength > 0) body = read;
    } catch {
      return errorResponse(400, "body_read_failed", "could not read request body");
    }
  }

  const upstreamHeaders = buildUpstreamHeaders(request.headers);
  if (hasRequestBody) {
    upstreamHeaders.set("content-length", String(body?.byteLength ?? 0));
  }

  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, config.timeoutMs);

  const onClientAbort = () => controller.abort();
  if (request.signal.aborted) {
    controller.abort();
  } else {
    request.signal.addEventListener("abort", onClientAbort, { once: true });
  }

  const cleanup = () => {
    clearTimeout(timer);
    request.signal.removeEventListener("abort", onClientAbort);
  };

  let upstreamResponse: Response;
  try {
    const init: RequestInit & { decompress: boolean } = {
      method: request.method,
      headers: upstreamHeaders,
      body,
      redirect: "manual",
      signal: controller.signal,
      // Không giải nén: body và Content-Encoding của upstream được chuyển
      // nguyên vẹn, không biến đổi. Đây là hành vi bắt buộc của proxy.
      decompress: false,
    };
    upstreamResponse = await fetch(target.toString(), init);
  } catch {
    cleanup();
    if (timedOut) {
      return errorResponse(504, "upstream_timeout", `upstream did not respond within ${config.timeoutMs} ms`);
    }
    return errorResponse(502, "upstream_unreachable", "could not reach upstream");
  }

  const responseHeaders = buildResponseHeaders(upstreamResponse.headers);

  if (upstreamResponse.body === null || NO_BODY_STATUS[upstreamResponse.status] === true) {
    cleanup();
    return new Response(null, {
      status: upstreamResponse.status,
      statusText: upstreamResponse.statusText,
      headers: responseHeaders,
    });
  }

  const streamedBody = relayBody(upstreamResponse.body, cleanup, () => controller.abort());
  return new Response(streamedBody, {
    status: upstreamResponse.status,
    statusText: upstreamResponse.statusText,
    headers: responseHeaders,
  });
}

function healthResponse(request: Request, config: RelayConfig): Response {
  if (request.method !== "GET" && request.method !== "HEAD") {
    return errorResponse(405, "method_not_allowed", "only GET /healthz is supported");
  }

  const payload = JSON.stringify({ ok: true, region: config.region, version: config.version });
  const headers = new Headers({
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-store",
    "content-length": String(byteLength(payload)),
  });

  return new Response(request.method === "HEAD" ? null : payload, { status: 200, headers });
}

function errorResponse(status: number, code: string, message: string): Response {
  const payload = JSON.stringify({ error: code, message });
  return new Response(payload, {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
      "content-length": String(byteLength(payload)),
    },
  });
}

function buildUpstreamHeaders(source: Headers): Headers {
  const connectionTokens = new Set(
    (source.get("connection") ?? "")
      .split(",")
      .map((value) => value.trim().toLowerCase())
      .filter(Boolean),
  );

  const headers = new Headers();
  for (const [name, value] of source) {
    const lower = name.toLowerCase();
    if (HOP_BY_HOP[lower] === true || connectionTokens.has(lower) || lower.startsWith("x-loimo-relay-")) continue;
    if (lower === "content-length") continue;
    headers.append(name, value);
  }
  return headers;
}

function buildResponseHeaders(source: Headers): Headers {
  const connectionTokens = new Set(
    (source.get("connection") ?? "")
      .split(",")
      .map((value) => value.trim().toLowerCase())
      .filter(Boolean),
  );
  const cookieSource = source as Headers & { getSetCookie?: () => string[] };
  const setCookies = typeof cookieSource.getSetCookie === "function" ? cookieSource.getSetCookie() : [];

  const headers = new Headers();
  for (const [name, value] of source) {
    const lower = name.toLowerCase();
    if (HOP_BY_HOP[lower] === true || connectionTokens.has(lower) || lower.startsWith("x-relay-")) continue;
    if (lower === "set-cookie") continue;
    // Content-Encoding và Content-Length được giữ nguyên vì body không bị
    // biến đổi (fetch với decompress: false).
    headers.append(name, value);
  }
  for (const cookie of setCookies) {
    headers.append("set-cookie", cookie);
  }
  return headers;
}

function relayBody(source: ReadableStream<Uint8Array>, cleanup: () => void, abortUpstream: () => void): ReadableStream<Uint8Array> {
  const reader = source.getReader();
  let settled = false;

  const settle = () => {
    if (settled) return;
    settled = true;
    cleanup();
  };

  return new ReadableStream<Uint8Array>({
    async pull(controller) {
      try {
        const { done, value } = await reader.read();
        if (done) {
          settle();
          controller.close();
          return;
        }
        controller.enqueue(value);
      } catch (error) {
        settle();
        controller.error(error);
      }
    },
    cancel(reason) {
      settle();
      abortUpstream();
      return reader.cancel(reason).catch(() => {});
    },
  });
}

const TOO_LARGE = Symbol("too_large");

async function readBody(request: Request, limit: number): Promise<ArrayBuffer | typeof TOO_LARGE> {
  if (!request.body) return new ArrayBuffer(0);

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;

  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > limit) {
        await reader.cancel().catch(() => undefined);
        return TOO_LARGE;
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const out = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return out.buffer as ArrayBuffer;
}

function constantTimeEquals(presented: string, expected: string): boolean {
  // Băm trước để thời gian so sánh không phụ thuộc độ dài chuỗi nhập vào.
  const presentedHash = createHash("sha256").update(presented, "utf8").digest();
  const expectedHash = createHash("sha256").update(expected, "utf8").digest();
  return timingSafeEqual(presentedHash, expectedHash) && presented.length === expected.length;
}

function clientEgress(headers: Headers): string {
  const forwardedFor = headers.get("x-forwarded-for")?.split(",")[0]?.trim();
  const candidate =
    headers.get("cf-connecting-ip")?.trim() ||
    forwardedFor ||
    headers.get("x-real-ip")?.trim() ||
    "";

  return candidate.replace(/[^0-9a-zA-Z.:\[\]]/g, "").slice(0, 100);
}

function normalizeAllowedHost(entry: string): string {
  const raw = entry.trim().toLowerCase();
  if (!raw) return "";
  try {
    // Chấp nhận cả "host", "host:port" và "https://host:port"; URL sẽ chuẩn
    // hoá chữ hoa, port mặc định và trailing dot.
    const parsed = new URL(raw.includes("://") ? raw : `https://${raw}`);
    return parsed.host.toLowerCase().replace(/\.$/, "");
  } catch {
    return "";
  }
}

function positiveInt(value: string | undefined, fallback: number): number {
  if (!value) return fallback;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function firstNonEmpty(...values: Array<string | undefined>): string | undefined {
  for (const value of values) {
    const trimmed = value?.trim();
    if (trimmed) return trimmed;
  }
  return undefined;
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function pathnameOf(rawUrl: string): string {
  try {
    return new URL(rawUrl).pathname.slice(0, 200);
  } catch {
    return "/";
  }
}
