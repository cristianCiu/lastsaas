import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [
    {
      name: 'local-app-name',
      apply: 'serve',
      transformIndexHtml: (html: string) => html.replace('{{APP_NAME}}', process.env.APP_NAME || 'LastSaaS'),
    },
    react(),
    tailwindcss(),
  ],
  server: {
    port: parseInt(process.env.VITE_PORT || '4280'),
    proxy: {
      '/api': {
        target: process.env.VITE_API_URL || 'http://localhost:4290',
        changeOrigin: true,
      },
    },
  },
})
