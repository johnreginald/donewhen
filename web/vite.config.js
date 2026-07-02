import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		port: 5173,
		proxy: {
			// Dev: proxy API + SSE + MCP to the Go server.
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true,
				// don't buffer SSE
				configure: (proxy) => {
					proxy.on('proxyRes', (proxyRes) => {
						proxyRes.headers['x-accel-buffering'] = 'no';
					});
				}
			},
			'/mcp': 'http://localhost:8080'
		}
	}
});
