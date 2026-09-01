import { useEffect, useState } from 'react'
import { api } from '../lib/api'

interface Props {
  src: string
  alt: string
  className?: string
}

/**
 * Photos are served behind bearer auth, so a plain <img src> would get a 401.
 * This fetches the bytes with the token and renders an object URL, revoking it
 * on unmount so a long gallery scroll doesn't leak blobs.
 */
export function AuthImage({ src, alt, className }: Props) {
  const [url, setUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let revoked = false
    let objectUrl = ''

    api
      .photoObjectURL(src)
      .then((u) => {
        if (revoked) {
          URL.revokeObjectURL(u)
          return
        }
        objectUrl = u
        setUrl(u)
      })
      .catch(() => setFailed(true))

    return () => {
      revoked = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [src])

  if (failed) {
    return (
      <div className={`flex items-center justify-center bg-ink-850 text-xs text-ink-500 ${className}`}>
        unavailable
      </div>
    )
  }

  if (!url) {
    return <div className={`animate-pulse bg-ink-850 ${className}`} />
  }

  return <img src={url} alt={alt} className={className} loading="lazy" />
}
