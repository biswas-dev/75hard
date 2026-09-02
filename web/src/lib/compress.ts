/**
 * Client-side image compression.
 *
 * A modern phone photo is 3-8MB. Sending that over a cellular connection is
 * the slowest part of logging a day, and the server downscales it to 1600px
 * anyway — so the bytes are shrunk here first, before they ever hit the wire.
 * The server repeats the work on what arrives, because a client can always
 * lie about what it sent.
 */

const MAX_EDGE = 1600
const QUALITY = 0.82

export interface CompressResult {
  blob: Blob
  originalBytes: number
  bytes: number
  width: number
  height: number
}

/** Compresses an image file, returning a WebP blob (JPEG where unsupported). */
export async function compressImage(file: Blob): Promise<CompressResult> {
  const bitmap = await loadBitmap(file)

  const { width, height } = fit(bitmap.width, bitmap.height, MAX_EDGE)
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height

  const ctx = canvas.getContext('2d')
  if (!ctx) {
    // No canvas means no compression; send the original rather than failing
    // the upload outright.
    return { blob: file, originalBytes: file.size, bytes: file.size, width: bitmap.width, height: bitmap.height }
  }

  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(bitmap, 0, 0, width, height)
  if ('close' in bitmap) bitmap.close()

  // Safari only gained WebP encoding in 16; fall back to JPEG when the
  // returned blob comes back with the wrong type.
  let blob = await toBlob(canvas, 'image/webp', QUALITY)
  if (!blob || blob.type !== 'image/webp') {
    blob = await toBlob(canvas, 'image/jpeg', QUALITY)
  }
  if (!blob) {
    return { blob: file, originalBytes: file.size, bytes: file.size, width, height }
  }

  // A tiny or already-optimised image can come out larger after re-encoding.
  if (blob.size >= file.size) {
    return { blob: file, originalBytes: file.size, bytes: file.size, width: bitmap.width, height: bitmap.height }
  }

  return { blob, originalBytes: file.size, bytes: blob.size, width, height }
}

async function loadBitmap(file: Blob): Promise<ImageBitmap | HTMLImageElement> {
  if ('createImageBitmap' in window) {
    try {
      // Honours the EXIF orientation flag, so a photo taken sideways is not
      // stored rotated.
      return await createImageBitmap(file, { imageOrientation: 'from-image' })
    } catch {
      // Fall through to the <img> path.
    }
  }

  const url = URL.createObjectURL(file)
  try {
    const img = new Image()
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve()
      img.onerror = () => reject(new Error('could not read that image'))
      img.src = url
    })
    return img
  } finally {
    // The decoded image keeps its own copy of the pixels.
    setTimeout(() => URL.revokeObjectURL(url), 0)
  }
}

function fit(w: number, h: number, edge: number) {
  if (w <= edge && h <= edge) return { width: w, height: h }
  return w >= h
    ? { width: edge, height: Math.round((h * edge) / w) }
    : { width: Math.round((w * edge) / h), height: edge }
}

function toBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, type, quality))
}

/** Human-readable byte size, for the "4.1 MB → 280 KB" upload feedback. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
