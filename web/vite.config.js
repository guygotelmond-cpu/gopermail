import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The API base URL is read at build/runtime from VITE_API_URL (defaults to
// apiserver's local port so `npm run dev` works out of the box against the
// docker-compose stack).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
  },
})
