import type { NextConfig } from "next";
import { fileURLToPath } from "node:url";

// Where the Go API runs. In development the browser talks to this app, and /api is passed
// on to the API, so cookies stay on one origin and no CORS setup is needed.
const apiOrigin = process.env.API_ORIGIN ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  outputFileTracingRoot: fileURLToPath(new URL(".", import.meta.url)),
  // Lets a production build for testing sit beside the running dev server.
  distDir: process.env.NEXT_DIST_DIR ?? ".next",

  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiOrigin}/api/:path*` }];
  },

  async headers() {
    return [
      {
        // The service worker must never be served from an old cache, or updates would not arrive.
        source: "/sw.js",
        headers: [
          { key: "Cache-Control", value: "no-cache, no-store, must-revalidate" },
          { key: "Service-Worker-Allowed", value: "/" },
        ],
      },
      {
        source: "/icons/:path*",
        headers: [{ key: "Cache-Control", value: "public, max-age=31536000, immutable" }],
      },
    ];
  },
};

export default nextConfig;
