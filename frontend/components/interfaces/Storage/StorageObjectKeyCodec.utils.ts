const STORAGE_SAFE_SEGMENT_REGEX = /^[A-Za-z0-9_!\-.*'() &$@=;:+,?]*$/
const ENCODED_SEGMENT_PREFIX = '__sb_u8__'
const ENCODED_SEGMENT_SUFFIX = '__'
const ENCODED_SEGMENT_REGEX = /^__sb_u8__([A-Za-z0-9_-]+)__$/
const NARROW_NO_BREAK_SPACE_REGEX = /\u{202F}/gu

function toBase64(input: string): string {
  const BufferCtor = (globalThis as any).Buffer
  if (typeof BufferCtor === 'function') {
    return BufferCtor.from(input, 'utf8').toString('base64')
  }

  const bytes = new TextEncoder().encode(input)
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })

  return btoa(binary)
}

function fromBase64(input: string): string {
  const BufferCtor = (globalThis as any).Buffer
  if (typeof BufferCtor === 'function') {
    return BufferCtor.from(input, 'base64').toString('utf8')
  }

  const binary = atob(input)
  const bytes = new Uint8Array(binary.length)
  for (let idx = 0; idx < binary.length; idx += 1) {
    bytes[idx] = binary.charCodeAt(idx)
  }

  return new TextDecoder().decode(bytes)
}

function toBase64URL(input: string): string {
  return toBase64(input).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/g, '')
}

function fromBase64URL(input: string): string {
  const base64 = input.replaceAll('-', '+').replaceAll('_', '/')
  const paddingLength = (4 - (base64.length % 4)) % 4
  const paddedBase64 = `${base64}${'='.repeat(paddingLength)}`
  return fromBase64(paddedBase64)
}

export function encodeStorageObjectSegment(segment: string): string {
  const normalizedSegment = segment.replaceAll(NARROW_NO_BREAK_SPACE_REGEX, ' ')
  if (normalizedSegment.length === 0) return normalizedSegment

  if (ENCODED_SEGMENT_REGEX.test(normalizedSegment)) return normalizedSegment
  if (STORAGE_SAFE_SEGMENT_REGEX.test(normalizedSegment)) return normalizedSegment

  return `${ENCODED_SEGMENT_PREFIX}${toBase64URL(normalizedSegment)}${ENCODED_SEGMENT_SUFFIX}`
}

export function decodeStorageObjectSegment(segment: string): string {
  const matches = segment.match(ENCODED_SEGMENT_REGEX)
  if (!matches) return segment

  try {
    return fromBase64URL(matches[1])
  } catch {
    return segment
  }
}

export function encodeStorageObjectPath(path: string): string {
  if (path.length === 0) return ''

  return path.split('/').map(encodeStorageObjectSegment).join('/')
}

export function decodeStorageObjectPath(path: string): string {
  if (path.length === 0) return ''

  return path.split('/').map(decodeStorageObjectSegment).join('/')
}
