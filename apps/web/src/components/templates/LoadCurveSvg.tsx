export function LoadCurveSvg({ path, name }: { path: string; name: string }) {
  const styleId = `draw-${name.replace(/[\s-]/g, '_').toLowerCase()}`
  return (
    <svg viewBox="0 0 200 80" className="h-20 w-full" aria-label={`${name} load curve`}>
      <style>{`
        @keyframes ${styleId} {
          0% { stroke-dashoffset: 600; }
          80% { stroke-dashoffset: 0; }
          100% { stroke-dashoffset: 0; }
        }
        .${styleId} {
          stroke-dasharray: 600;
          stroke-dashoffset: 600;
          animation: ${styleId} 4s ease-in-out infinite;
        }
      `}</style>
      <rect width="200" height="80" rx="4" className="fill-slate-950" />
      {/* Grid lines */}
      <line x1="10" y1="70" x2="190" y2="70" className="stroke-slate-800" strokeWidth="0.5" />
      <line x1="10" y1="40" x2="190" y2="40" className="stroke-slate-800" strokeWidth="0.5" />
      <line x1="10" y1="10" x2="190" y2="10" className="stroke-slate-800" strokeWidth="0.5" />
      {/* Load curve */}
      <path
        d={path}
        fill="none"
        className={`${styleId} stroke-teal-500`}
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {name === 'Breakpoint' && (
        <>
          <line x1="155" y1="10" x2="165" y2="20" className="stroke-red-500" strokeWidth="2.5" strokeLinecap="round" />
          <line x1="165" y1="10" x2="155" y2="20" className="stroke-red-500" strokeWidth="2.5" strokeLinecap="round" />
        </>
      )}
    </svg>
  )
}
