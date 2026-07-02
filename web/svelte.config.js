import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// SPA: no server, fall back to index.html for client routing.
		adapter: adapter({ fallback: 'index.html', strict: false }),
		alias: { $components: 'src/components' }
	}
};

export default config;
