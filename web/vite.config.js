import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Where the Go server listens. Override when :8080 is taken on this machine.
const api = process.env.DONEWHEN_API || 'http://localhost:8080';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		port: 5173,
		proxy: {
			// Dev: proxy API + SSE + MCP to the Go server.
			'/api': {
				target: api,
				changeOrigin: true,
				// don't buffer SSE
				configure: (proxy) => {
					proxy.on('proxyRes', (proxyRes) => {
						proxyRes.headers['x-accel-buffering'] = 'no';
					});
				}
			},
			'/mcp': api
		}
	}
});
