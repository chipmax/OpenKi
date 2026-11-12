import { gunzipSync, gzipSync } from "node:zlib";
import { createRelayHandler, loadConfig, type RelayConfig } from "../src/app";

const TOKEN = "test-token-7d1f0c";
const failures: string[] = [];
let passed = 0;

function check(name: string, condition: boolean, detail = ""): void {
  if (condition) {
    passed += 1;
    console.log(`ok - ${name}`);
    return;
  }
  const message = detail ? `${name}: ${detail}` : name;
  failures.push(message);
  console.error(`not ok - ${message}`);
}

function makeConfig(overrides: Partial<RelayConfig> = {}): RelayConfig {
  return {
    token: TOKEN,
    allowedHosts: new Set(["127.0.0.1"]),
    allowHttp: true,
    region: "test-region",
    version: "test-version",
    maxBodyBytes: 1024 * 1024,
    timeoutMs: 5_000,
    ...overrides,
  };
}

const jsonHeaders = { "content-type": "application/json" };

// Upstream echo nội bộ, không dùng mạng ngoài.
const upstream = Bun.serve({
  port: 0,
  hostname: "127.0.0.1",
  async fetch(request) {
    const url = new URL(request.url);

    if (url.pathname === "/v1/sse") {
      const encoder = new TextEncoder();
      return new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(encoder.encode("data: first\n\n"));
            setTimeout(() => {
              controller.enqueue(encoder.encode("data: second\n\n"));
              controller.close();
            }, 120);
          },
        }),
        { headers: { "content-type": "text/event-stream", "cache-control": "no-cache" } },
      );
    }

    if (url.pathname === "/v1/slow") {
      await Bun.sleep(1_000);
      return new Response("late", { headers: jsonHeaders });
    }

    if (url.pathname === "/v1/encoded") {
      const gzipped = gzipSync(JSON.stringify({ encoded: true }));
      return new Response(gzipped, {
        headers: { "content-type": "application/json", "content-encoding": "gzip" },
      });
    }

    const body = request.method === "GET" || request.method === "HEAD" ? "" : await request.text();
    return Response.json({
      method: request.method,
      path: url.pathname,
      query: url.search,
      body,
      headers: Object.fromEntries(request.headers),
    });
  },
});

const relay = Bun.serve({
  port: 0,
  hostname: "127.0.0.1",
  idleTimeout: 255,
  fetch: createRelayHandler(makeConfig()),
});

const relayBase = `http://127.0.0.1:${relay.port}`;
const upstreamOrigin = `http://127.0.0.1:${upstream.port}`;

function relayHeaders(extra: Record<string, string> = {}): Record<string, string> {
  return {
    "x-openki-relay-token": TOKEN,
    "x-openki-relay-upstream": upstreamOrigin,
    ...extra,
  };
}

async function run(): Promise<void> {
  const health = await fetch(`${relayBase}/healthz`);
  const healthBody = (await health.json()) as { ok: boolean; region: string; version: string };
  check("healthz status", health.status === 200, String(health.status));
  check("healthz body", healthBody.ok === true && healthBody.region === "test-region" && healthBody.version === "test-version");
  check("healthz region header", health.headers.get("x-relay-region") === "test-region");

  const healthPost = await fetch(`${relayBase}/healthz`, { method: "POST" });
  check("healthz rejects POST", healthPost.status === 405, String(healthPost.status));

  const noToken = await fetch(`${relayBase}/v1/models`, { headers: { "x-openki-relay-upstream": upstreamOrigin } });
  check("missing token rejected", noToken.status === 401, String(noToken.status));

  const wrongToken = await fetch(`${relayBase}/v1/models`, {
    headers: { "x-openki-relay-token": "wrong", "x-openki-relay-upstream": upstreamOrigin },
  });
  check("wrong token rejected", wrongToken.status === 401, String(wrongToken.status));

  const blockedHost = await fetch(`${relayBase}/v1/models`, {
    headers: { "x-openki-relay-token": TOKEN, "x-openki-relay-upstream": `http://localhost:${upstream.port}` },
  });
  check("host outside allowlist rejected", blockedHost.status === 403, String(blockedHost.status));

  const missingUpstream = await fetch(`${relayBase}/v1/models`, {
    headers: { "x-openki-relay-token": TOKEN },
  });
  check("missing upstream rejected", missingUpstream.status === 400, String(missingUpstream.status));

  const malformedUpstream = await fetch(`${relayBase}/v1/models`, {
    headers: { "x-openki-relay-token": TOKEN, "x-openki-relay-upstream": "not a url" },
  });
  check("malformed upstream rejected", malformedUpstream.status === 400, String(malformedUpstream.status));

  const pathInUpstream = await fetch(`${relayBase}/v1/models`, {
    headers: { "x-openki-relay-token": TOKEN, "x-openki-relay-upstream": `${upstreamOrigin}/v1` },
  });
  check("upstream with path rejected", pathInUpstream.status === 400, String(pathInUpstream.status));

  const emptyAllowlist = createRelayHandler(makeConfig({ allowedHosts: new Set() }));
  const emptyAllowlistRes = await emptyAllowlist(
    new Request("http://relay.local/v1/models", { headers: relayHeaders() }),
  );
  check("empty allowlist fails closed", emptyAllowlistRes.status === 403, String(emptyAllowlistRes.status));

  const httpsOnly = createRelayHandler(makeConfig({ allowHttp: false }));
  const httpRes = await httpsOnly(new Request("http://relay.local/v1/models", { headers: relayHeaders() }));
  check("http upstream rejected without opt-in", httpRes.status === 403, String(httpRes.status));

  const echo = await fetch(`${relayBase}/v1/echo?a=1&b=two`, {
    headers: relayHeaders({
      authorization: "Bearer upstream-key",
      "x-api-key": "anthropic-key",
      "anthropic-version": "2023-06-01",
      "x-openki-relay-debug": "must-be-stripped",
      "cf-connecting-ip": "203.0.113.9",
    }),
  });
  const echoBody = (await echo.json()) as {
    method: string;
    path: string;
    query: string;
    headers: Record<string, string>;
  };
  check("proxy status", echo.status === 200, String(echo.status));
  check("method preserved", echoBody.method === "GET");
  check("path preserved", echoBody.path === "/v1/echo", echoBody.path);
  check("query preserved", echoBody.query === "?a=1&b=two", echoBody.query);
  check("authorization forwarded", echoBody.headers.authorization === "Bearer upstream-key");
  check("x-api-key forwarded", echoBody.headers["x-api-key"] === "anthropic-key");
  check("anthropic-version forwarded", echoBody.headers["anthropic-version"] === "2023-06-01");
  check(
    "x-openki-relay-* stripped",
    !Object.keys(echoBody.headers).some((name) => name.startsWith("x-openki-relay-")),
  );
  check("host rewritten to upstream", echoBody.headers.host === `127.0.0.1:${upstream.port}`, echoBody.headers.host);
  check("region response header", echo.headers.get("x-relay-region") === "test-region");
  check("egress response header", echo.headers.get("x-relay-egress") === "203.0.113.9");

  const payload = JSON.stringify({ hello: "việt", stream: false });
  const post = await fetch(`${relayBase}/v1/chat?stream=false`, {
    method: "POST",
    headers: relayHeaders({ "content-type": "application/json" }),
    body: payload,
  });
  const postBody = (await post.json()) as { method: string; path: string; body: string };
  check("POST method preserved", postBody.method === "POST");
  check("POST path preserved", postBody.path === "/v1/chat", postBody.path);
  check("POST body preserved", postBody.body === payload);

  const arbitrary = await fetch(`${relayBase}/custom/path?z=1`, { headers: relayHeaders() });
  const arbitraryBody = (await arbitrary.json()) as { path: string; query: string };
  check("non-/v1 path proxied", arbitrary.status === 200 && arbitraryBody.path === "/custom/path");
  check("non-/v1 query preserved", arbitraryBody.query === "?z=1", arbitraryBody.query);

  const sse = await fetch(`${relayBase}/v1/sse`, {
    headers: relayHeaders({ accept: "text/event-stream" }),
  });
  check("sse status", sse.status === 200, String(sse.status));
  check("sse content-type", (sse.headers.get("content-type") ?? "").includes("text/event-stream"));

  if (sse.body) {
    const reader = sse.body.getReader();
    const decoder = new TextDecoder();
    const first = await reader.read();
    const firstText = first.done ? "" : decoder.decode(first.value, { stream: true });
    check("sse first event arrives", firstText.includes("data: first"), firstText);
    check("sse stream is unbuffered", !firstText.includes("data: second"), "second event arrived in the first chunk");

    let rest = "";
    for (;;) {
      const next = await reader.read();
      if (next.done) break;
      rest += decoder.decode(next.value, { stream: true });
    }
    check("sse second event arrives", rest.includes("data: second"), rest);
  } else {
    check("sse has a body", false);
  }

  const encodedInit: RequestInit & { decompress: boolean } = {
    headers: relayHeaders({ "accept-encoding": "gzip" }),
    decompress: false,
  };
  const encoded = await fetch(`${relayBase}/v1/encoded`, encodedInit);
  const encodedBytes = new Uint8Array(await encoded.arrayBuffer());
  check("content-encoding preserved", encoded.headers.get("content-encoding") === "gzip");
  check(
    "compressed body passed through",
    encodedBytes[0] === 0x1f && encodedBytes[1] === 0x8b,
    `first bytes: ${encodedBytes[0]}, ${encodedBytes[1]}`,
  );
  const decoded = JSON.parse(gunzipSync(Buffer.from(encodedBytes)).toString("utf8")) as { encoded: boolean };
  check("gzip payload intact", decoded.encoded === true);

  const dead = Bun.serve({ port: 0, hostname: "127.0.0.1", fetch: () => new Response("x") });
  const deadPort = dead.port;
  await dead.stop(true);
  const unreachable = createRelayHandler(makeConfig());
  const unreachableRes = await unreachable(
    new Request("http://relay.local/v1/models", {
      headers: { "x-openki-relay-token": TOKEN, "x-openki-relay-upstream": `http://127.0.0.1:${deadPort}` },
    }),
  );
  check("unreachable upstream returns 502", unreachableRes.status === 502, String(unreachableRes.status));

  const slow = createRelayHandler(makeConfig({ timeoutMs: 100 }));
  const slowRes = await slow(
    new Request("http://relay.local/v1/slow", { headers: relayHeaders() }),
  );
  check("timeout returns 504", slowRes.status === 504, String(slowRes.status));

  const tinyLimit = createRelayHandler(makeConfig({ maxBodyBytes: 16 }));
  const tooLargeRes = await tinyLimit(
    new Request("http://relay.local/v1/chat", {
      method: "POST",
      headers: relayHeaders({ "content-type": "application/json" }),
      body: "x".repeat(64),
    }),
  );
  check("oversized body returns 413", tooLargeRes.status === 413, String(tooLargeRes.status));

  const parsed = loadConfig({
    OPENKI_RELAY_TOKEN: "t",
    OPENKI_RELAY_HOSTS: "opencode.ai, https://API.Example.com:8443, ",
    OPENKI_RELAY_ALLOW_HTTP: "1",
    OPENKI_RELAY_MAX_BODY_BYTES: "2048",
    OPENKI_RELAY_TIMEOUT_MS: "1234",
    VERCEL_REGION: "sin1",
  });
  check(
    "env parsing",
    parsed.token === "t" &&
      parsed.allowedHosts.has("opencode.ai") &&
      parsed.allowedHosts.has("api.example.com:8443") &&
      parsed.allowHttp &&
      parsed.maxBodyBytes === 2048 &&
      parsed.timeoutMs === 1234 &&
      parsed.region === "sin1",
  );
}

try {
  await run();
} catch (error) {
  failures.push(`unexpected error: ${error instanceof Error ? error.message : String(error)}`);
} finally {
  await relay.stop(true);
  await upstream.stop(true);
}

console.log(`\n${passed} passed, ${failures.length} failed`);
if (failures.length > 0) {
  for (const failure of failures) console.error(` - ${failure}`);
  process.exit(1);
}
