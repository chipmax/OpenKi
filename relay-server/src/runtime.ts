import { createRelayHandler, DEFAULT_PORT, loadConfig, VERSION } from "./app";

/**
 * Khởi động HTTP server. Được gọi một lần khi module entry load, đúng yêu cầu
 * của Vercel Bun runtime (preset Bun.serve).
 */
export function startServer(): void {
  const config = loadConfig();

  const parsedPort = Number(process.env.PORT ?? DEFAULT_PORT);
  const port = Number.isSafeInteger(parsedPort) && parsedPort > 0 && parsedPort <= 65535 ? parsedPort : DEFAULT_PORT;
  const hostname = process.env.HOST?.trim() || "0.0.0.0";

  if (!config.token) {
    console.warn("[relay] OPENKI_RELAY_TOKEN is empty: every proxy request will be rejected (fail closed)");
  }
  if (config.allowedHosts.size === 0) {
    console.warn("[relay] OPENKI_RELAY_HOSTS is empty: every upstream host will be rejected (fail closed)");
  }

  const server = Bun.serve({
    port,
    hostname,
    idleTimeout: 255,
    maxRequestBodySize: Math.min(config.maxBodyBytes + 1024 * 1024, 1024 * 1024 * 1024),
    fetch: createRelayHandler(config),
  });

  console.log(
    JSON.stringify({
      level: "info",
      msg: "relay_listening",
      hostname: server.hostname,
      port: server.port,
      region: config.region,
      version: VERSION,
      bun: (process.versions as Record<string, string | undefined>).bun ?? "unknown",
    }),
  );
}
