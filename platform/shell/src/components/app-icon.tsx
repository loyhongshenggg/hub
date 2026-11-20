import { cn } from '@hub/ui-kit';
import { useState } from 'react';
import type { AppInfo } from '../api/types';

export function hue(s: string): number {
  let h = 0;
  for (const c of s) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return h % 360;
}

/** The manifest icon, or a coloured tile with the app's initial. */
export function AppIcon({
  app,
  size = 36,
  className,
}: {
  app: Pick<AppInfo, 'id' | 'name' | 'icon'>;
  size?: number;
  className?: string;
}) {
  const [broken, setBroken] = useState(false);
  const box = { width: size, height: size };
  if (app.icon && !broken) {
    return (
      <img
        src={app.icon}
        alt=""
        draggable={false}
        style={box}
        onError={() => setBroken(true)}
        className={cn('shrink-0 rounded-[24%] object-cover', className)}
      />
    );
  }
  return (
    <div
      aria-hidden
      style={{
        ...box,
        background: `oklch(0.62 0.14 ${hue(app.id)})`,
        fontSize: size * 0.46,
      }}
      className={cn(
        'grid shrink-0 select-none place-items-center rounded-[24%] font-semibold text-white',
        className,
      )}
    >
      {app.name.charAt(0).toUpperCase()}
    </div>
  );
}
