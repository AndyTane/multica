import type { MetadataRoute } from "next";
import { resolvePublicAppUrl } from "../lib/public-host";

export default function robots(): MetadataRoute.Robots {
  const baseUrl = resolvePublicAppUrl(process.env);

  return {
    rules: [
      {
        userAgent: "*",
        allow: ["/", "/about", "/changelog"],
        disallow: [
          "/api/",
          "/ws",
          "/auth/",
          "/issues",
          "/board",
          "/inbox",
          "/agents",
          "/settings",
          "/my-issues",
          "/runtimes",
          "/skills",
        ],
      },
    ],
    ...(baseUrl ? { sitemap: [`${baseUrl}/sitemap.xml`] } : {}),
  };
}
