/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';

export default defineConfig({
  base: './',
  plugins: [
    react(),
    VitePWA({
      registerType: 'prompt',
      includeAssets: ['icons/*.png'],
      manifest: {
        name: 'NAS Gallery',
        short_name: 'NAS Gallery',
        description: 'Self-hosted photo and video gallery for your NAS',
        theme_color: '#0d1016',
        background_color: '#0d1016',
        display: 'standalone',
        start_url: './',
        scope: './',
        icons: [
          { src: 'icons/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: 'icons/icon-512.png', sizes: '512x512', type: 'image/png' },
          { src: 'icons/maskable-192.png', sizes: '192x192', type: 'image/png', purpose: 'maskable' },
          { src: 'icons/maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,png,woff2}'],
        // Hash router: navigations never hit the network path, so no
        // navigateFallback is needed; and /api must never be cached anyway.
        navigateFallbackDenylist: [/\/api\//],
        runtimeCaching: [
          {
            // Signed thumbnail URLs. CacheFirst per the spec (~3000 entries,
            // 30 days). Signatures rotate per API response, so entries are
            // keyed by full URL and evicted LRU.
            urlPattern: /\/api\/media\/[^/]+\/thumb/,
            handler: 'CacheFirst',
            options: {
              cacheName: 'nas-gallery-thumbs',
              expiration: { maxEntries: 3000, maxAgeSeconds: 60 * 60 * 24 * 30 },
              cacheableResponse: { statuses: [200] },
            },
          },
        ],
      },
    }),
  ],
  build: {
    target: 'es2020',
    sourcemap: false,
  },
  test: {
    environment: 'node',
    setupFiles: ['src/test/setup.ts'],
    include: ['src/**/*.test.ts'],
  },
});
