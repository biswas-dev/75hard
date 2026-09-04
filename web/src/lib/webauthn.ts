/**
 * Browser-side WebAuthn plumbing.
 *
 * The credential API speaks ArrayBuffers and the wire speaks base64url, so
 * every ceremony needs the same translation in both directions. It lives here
 * rather than in the components because getting it subtly wrong produces a
 * signature that fails to verify with no useful message anywhere.
 */

/** Decode base64url — no padding, and the two substituted characters. */
export function fromBase64url(value: string): Uint8Array {
  const padded = value.replace(/-/g, '+').replace(/_/g, '/')
  const binary = atob(padded + '='.repeat((4 - (padded.length % 4)) % 4))
  const out = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i)
  return out
}

/** Encode base64url, which is what the server expects back. */
export function toBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/** Whether this browser can do WebAuthn at all. */
export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential
}

/**
 * Whether the device itself can act as an authenticator — Touch ID, Windows
 * Hello, an Android screen lock.
 *
 * Used to word the offer honestly: "use Touch ID" on a laptop that has it, and
 * a plainer "use a passkey" where the key will come from a phone or a
 * security key.
 */
export async function platformAuthenticatorAvailable(): Promise<boolean> {
  if (!passkeysSupported()) return false
  try {
    return await window.PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable()
  } catch {
    return false
  }
}

type Options = Record<string, unknown>

/**
 * Turn the server's creation options into what navigator.credentials wants.
 *
 * Only the fields that are genuinely binary are converted; everything else is
 * passed through, so a future option the server starts sending arrives intact
 * without a change here.
 */
export function decodeCreationOptions(options: Options): PublicKeyCredentialCreationOptions {
  const o = { ...options } as any
  o.challenge = fromBase64url(o.challenge)
  o.user = { ...o.user, id: fromBase64url(o.user.id) }
  if (Array.isArray(o.excludeCredentials)) {
    o.excludeCredentials = o.excludeCredentials.map((c: any) => ({
      ...c,
      id: fromBase64url(c.id),
    }))
  }
  return o as PublicKeyCredentialCreationOptions
}

/** The same, for a sign-in assertion. */
export function decodeRequestOptions(options: Options): PublicKeyCredentialRequestOptions {
  const o = { ...options } as any
  o.challenge = fromBase64url(o.challenge)
  if (Array.isArray(o.allowCredentials)) {
    o.allowCredentials = o.allowCredentials.map((c: any) => ({
      ...c,
      id: fromBase64url(c.id),
    }))
  }
  return o as PublicKeyCredentialRequestOptions
}

/** Serialise a newly created credential for the server. */
export function encodeCreatedCredential(cred: PublicKeyCredential): unknown {
  const res = cred.response as AuthenticatorAttestationResponse
  return {
    id: cred.id,
    rawId: toBase64url(cred.rawId),
    type: cred.type,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: toBase64url(res.clientDataJSON),
      attestationObject: toBase64url(res.attestationObject),
      transports: typeof res.getTransports === 'function' ? res.getTransports() : [],
    },
  }
}

/** Serialise an assertion for the server. */
export function encodeAssertion(cred: PublicKeyCredential): unknown {
  const res = cred.response as AuthenticatorAssertionResponse
  return {
    id: cred.id,
    rawId: toBase64url(cred.rawId),
    type: cred.type,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: toBase64url(res.clientDataJSON),
      authenticatorData: toBase64url(res.authenticatorData),
      signature: toBase64url(res.signature),
      userHandle: res.userHandle ? toBase64url(res.userHandle) : null,
    },
  }
}

/**
 * A cancelled prompt is not an error worth showing.
 *
 * Dismissing the browser's dialog throws NotAllowedError, the same name a
 * genuine refusal uses; treating it as a failure would put a red message on
 * screen every time someone changed their mind.
 */
export function wasCancelled(err: unknown): boolean {
  return err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'AbortError')
}
