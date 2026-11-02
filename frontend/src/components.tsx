import { useLayoutEffect, useRef, useState, type CSSProperties, type HTMLAttributes, type ReactNode } from 'react'

// Pill/segment groups: slide one indicator to the selected button instead of repainting each button.
export function SelectionTrack({ active, children, className = '', ...props }: HTMLAttributes<HTMLDivElement> & { active: string }) {
 const track = useRef<HTMLDivElement>(null)
 const [position, setPosition] = useState<{ x: number; y: number; width: number; height: number } | null>(null)
 useLayoutEffect(() => {
  const root = track.current
  if (!root) return
  const measure = () => {
   const button = root.querySelector<HTMLButtonElement>('button[aria-selected="true"], button[aria-checked="true"]')
   if (button) setPosition({ x: button.offsetLeft, y: button.offsetTop, width: button.offsetWidth, height: button.offsetHeight })
  }
  measure()
  const observer = new ResizeObserver(measure)
  observer.observe(root)
  root.querySelectorAll('button').forEach(button => observer.observe(button))
  return () => observer.disconnect()
 }, [active])
 const style = position ? { '--selection-x': `${position.x}px`, '--selection-y': `${position.y}px`, '--selection-width': `${position.width}px`, '--selection-height': `${position.height}px` } as CSSProperties : undefined
 return <div {...props} ref={track} className={`selection-track ${className}`} style={style}>
  <span className="selection-indicator" aria-hidden="true" data-ready={!!position} />{children}
 </div>
}

export function Switch({ checked, onChange, label, disabled = false }: { checked: boolean; onChange: (value: boolean) => void; label: string; disabled?: boolean }) {
 return <button type="button" role="switch" aria-label={label} aria-checked={checked} disabled={disabled} onClick={() => onChange(!checked)} className={`switch ${checked ? 'on' : ''}`}><span /></button>
}
export function FeatureCard({ icon, title, desc, checked, onChange, disabled, wide, children }: { icon: ReactNode; title: string; desc: string; checked: boolean; onChange: (v: boolean) => void; disabled: boolean; wide?: boolean; children?: ReactNode }) {
 return <div className={`feature-card ${checked ? 'on' : ''} ${wide ? 'wide' : ''}`}>
  <div className="feature-top"><span className="badge">{icon}</span><Switch checked={checked} onChange={onChange} label={title} disabled={disabled} /></div>
  <strong>{title}</strong><p>{desc}</p>
  {children && <div className="feature-extra">{children}</div>}
 </div>
}
export function OptionRow({ title, desc, control }: { title: string; desc: string; control: ReactNode }) {
 return <div className="row"><div><strong>{title}</strong><small>{desc}</small></div>{control}</div>
}
