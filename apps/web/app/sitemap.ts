import type { MetadataRoute } from "next";
import { resolvePublicAppUrl } from "../lib/public-host";

export default function sitemap(): MetadataRoute.Sitemap {
  const baseUrl = resolvePublicAppUrl(process.env);
  if (!baseUrl) return [];

  return [
    {
      url: baseUrl,
      lastModified: new Date("2026-04-01"),
      changeFrequency: "weekly",
      priority: 1.0,
    },
    {
      url: `${baseUrl}/about`,
      lastModified: new Date("2026-04-01"),
      changeFrequency: "monthly",
      priority: 0.7,
    },
    {
      url: `${baseUrl}/changelog`,
      lastModified: new Date("2026-04-01"),
      changeFrequency: "weekly",
      priority: 0.6,
    },
    {
      url: `${baseUrl}/contact-sales`,
      lastModified: new Date("2026-05-21"),
      changeFrequency: "monthly",
      priority: 0.7,
    },
    {
      url: `${baseUrl}/licensing`,
      lastModified: new Date("2026-09-21"),
      changeFrequency: "monthly",
      priority: 0.6,
    },
    {
      url: `${baseUrl}/privacy`,
      lastModified: new Date("2026-09-21"),
      changeFrequency: "yearly",
      priority: 0.3,
    },
  ];
}
