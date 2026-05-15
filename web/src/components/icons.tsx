// Small inline SVG icons.

export function LeafLogo({ size = 22 }: { size?: number }) {
  // A simplified MongoDB-style leaf mark.
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M12 1.5c2.6 3.2 5.3 6.4 5.3 12 0 4.6-2.7 7.2-4.6 8.4l-.4.3-.3-.3C10 22.4 7.7 19.8 7.7 13.5c0-5.6 1.7-8.8 4.3-12z"
        fill="#00ED64"
      />
      <path
        d="M12 1.5c2.6 3.2 5.3 6.4 5.3 12 0 4.6-2.7 7.2-4.6 8.4l-.4.3-.3-7.5z"
        fill="#00A35C"
      />
      <path d="M12 22.5l.3-.3v2.3H12z" fill="#00684A" />
    </svg>
  )
}
