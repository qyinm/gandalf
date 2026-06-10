import { defineCollection, z } from "astro:content";

const docsCollection = defineCollection({
	schema: z.object({
		title: z.string(),
		description: z.string().optional(),
		order: z.number().optional(),
		category: z.string().optional(),
	}),
});

export const collections = {
	docs: docsCollection,
};
