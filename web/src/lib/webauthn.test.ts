import { describe, expect, it } from 'vitest'
import {
  decodeCreationOptions,
  decodeRequestOptions,
  fromBase64url,
  toBase64url,
} from './webauthn'

/**
 * A byte wrong anywhere here produces a signature the server cannot verify,
 * with no useful message on either side. It is worth pinning down.
 */
describe('base64url', () => {
  it('round-trips bytes', () => {
    const bytes = new Uint8Array([0, 1, 2, 250, 251, 252, 253, 254, 255])
    expect(new Uint8Array(fromBase64url(toBase64url(bytes.buffer)))).toEqual(bytes)
  })

  it('decodes the characters standard base64 does not use', () => {
    // 0xfb 0xff encodes as "-_8" in base64url and "+/8" in base64; reading it
    // with the wrong alphabet silently yields different bytes.
    expect(Array.from(fromBase64url('-_8'))).toEqual([251, 255])
  })

  it('handles every unpadded length', () => {
    for (const n of [1, 2, 3, 4, 5]) {
      const bytes = new Uint8Array(n).fill(7)
      expect(fromBase64url(toBase64url(bytes.buffer)).length).toBe(n)
    }
  })

  it('never emits padding or the substituted characters', () => {
    const encoded = toBase64url(new Uint8Array([251, 255, 254]).buffer)
    expect(encoded).not.toMatch(/[+/=]/)
  })
})

describe('ceremony options', () => {
  it('converts the challenge and user handle to bytes', () => {
    const out = decodeCreationOptions({
      challenge: 'AQID',
      user: { id: 'BAUG', name: 'a@b.c' },
      rp: { name: '75 Hard' },
    })
    expect(Array.from(new Uint8Array(out.challenge as ArrayBuffer))).toEqual([1, 2, 3])
    expect(Array.from(new Uint8Array(out.user.id as ArrayBuffer))).toEqual([4, 5, 6])
    // Untouched fields survive, so a new option the server sends still arrives.
    expect(out.rp.name).toBe('75 Hard')
  })

  it('converts excluded credential ids', () => {
    const out = decodeCreationOptions({
      challenge: 'AQID',
      user: { id: 'BAUG' },
      excludeCredentials: [{ id: 'AQID', type: 'public-key' }],
    })
    expect(Array.from(new Uint8Array(out.excludeCredentials![0].id as ArrayBuffer))).toEqual([1, 2, 3])
  })

  it('leaves a discoverable sign-in without an allow list', () => {
    // A passwordless sign-in sends no allowCredentials at all; inventing an
    // empty one would stop the browser offering any passkey.
    const out = decodeRequestOptions({ challenge: 'AQID' })
    expect(out.allowCredentials).toBeUndefined()
    expect(Array.from(new Uint8Array(out.challenge as ArrayBuffer))).toEqual([1, 2, 3])
  })
})
