import type { MetadataRoute } from "next";
/** Private application: no public marketing pages are indexed. */
export default function sitemap(): MetadataRoute.Sitemap { return []; }
