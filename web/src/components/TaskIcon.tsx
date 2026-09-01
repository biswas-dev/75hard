/**
 * Inline SVG icons for the task template. Hand-drawn rather than pulled from
 * an icon package so the bundle carries only the handful actually used.
 */
const paths: Record<string, JSX.Element> = {
  dumbbell: (
    <>
      <path d="M6.5 6.5v11M17.5 6.5v11M3 9v6M21 9v6M6.5 12h11" />
    </>
  ),
  tree: (
    <>
      <path d="M12 22v-6" />
      <path d="M12 2 6 10h3l-4 6h14l-4-6h3L12 2Z" />
    </>
  ),
  salad: (
    <>
      <path d="M7 21h10a5 5 0 0 0 5-5H2a5 5 0 0 0 5 5Z" />
      <path d="M12 11a3 3 0 1 0-3-3M15 8a3 3 0 0 1 3 3" />
    </>
  ),
  droplet: <path d="M12 2.7 6.6 8.1a7.6 7.6 0 1 0 10.8 0L12 2.7Z" />,
  book: (
    <>
      <path d="M4 4.5A2.5 2.5 0 0 1 6.5 2H20v18H6.5A2.5 2.5 0 0 0 4 22.5Z" />
      <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
    </>
  ),
  camera: (
    <>
      <path d="M3 8h3l2-3h8l2 3h3v11H3z" />
      <circle cx="12" cy="13" r="3.5" />
    </>
  ),
  check: <path d="M20 6 9 17l-5-5" />,
  flame: <path d="M12 2c3 4 6 6 6 10a6 6 0 1 1-12 0c0-2 1-3 2-4 0 2 1 3 2 3 0-3 1-6 2-9Z" />,
  moon: <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />,
  run: (
    <>
      <circle cx="15" cy="4.5" r="2" />
      <path d="M8 21l3-6 4 2 1 4M6 12l4-3 3 3 3 1" />
    </>
  ),
  pill: (
    <>
      <rect x="2" y="8" width="20" height="8" rx="4" />
      <path d="M12 8v8" />
    </>
  ),
  scale: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="3" />
      <path d="M12 8v3M8.5 8.5 12 11l3.5-2.5" />
    </>
  ),
}

export function TaskIcon({ name, size = 22 }: { name: string; size?: number }) {
  const path = paths[name] ?? paths.check
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      {path}
    </svg>
  )
}

/** The icon names offered in the task editor. */
export const ICON_NAMES = Object.keys(paths)
