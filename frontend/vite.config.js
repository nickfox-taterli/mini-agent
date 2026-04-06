import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

function serveUploadDir() {
  return {
    name: 'serve-upload-dir',
    configureServer(server) {
      server.middlewares.use('/upload', (req, res, next) => {
        const filePath = path.join(__dirname, 'upload', req.url)
        if (!fs.existsSync(filePath) || !fs.statSync(filePath).isFile()) {
          return next()
        }
        const ext = path.extname(filePath).toLowerCase()
        const mimeTypes = {
          '.xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          '.xls': 'application/vnd.ms-excel',
          '.pdf': 'application/pdf',
          '.png': 'image/png',
          '.jpg': 'image/jpeg',
          '.jpeg': 'image/jpeg',
          '.gif': 'image/gif',
          '.webp': 'image/webp',
          '.svg': 'image/svg+xml',
          '.txt': 'text/plain',
          '.csv': 'text/csv',
          '.json': 'application/json',
          '.zip': 'application/zip',
          '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
          '.doc': 'application/msword',
          '.mp4': 'video/mp4',
          '.webm': 'video/webm',
        }
        const contentType = mimeTypes[ext] || 'application/octet-stream'
        res.setHeader('Content-Type', contentType)
        res.setHeader('Content-Disposition', `inline; filename="${path.basename(filePath)}"`)
        fs.createReadStream(filePath).pipe(res)
      })
    }
  }
}

export default defineConfig({
  plugins: [vue(), serveUploadDir()],
  server: {
    host: '0.0.0.0',
    port: 18889
  }
})
