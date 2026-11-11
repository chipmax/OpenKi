import { startServer } from "./src/runtime";

// Vercel Bun preset yêu cầu Bun.serve() được gọi đúng một lần khi module
// entry load. File này là entrypoint của preset (project root = relay-server).
startServer();
