import { defineConfig } from "astro/config";
import mdx from "@astrojs/mdx";
import react from "@astrojs/react";
import sitemap from "@astrojs/sitemap";

export default defineConfig({
	site: "https://usehem.com",
	integrations: [mdx(), react(), sitemap()],
	output: "static",
	build: {
		assets: "_assets",
	},
});
