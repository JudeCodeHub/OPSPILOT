/** Inline loading spinner with a visible label, for buttons. */
export function Spinner({ label }: { label: string }) {
  return (
    <>
      <svg className="size-4 animate-spin" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeOpacity="0.25" strokeWidth="3" />
        <path d="M21 12a9 9 0 0 0-9-9" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
      </svg>
      {label}
    </>
  )
}
