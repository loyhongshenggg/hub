// Radar's look: a colour and an icon per skill area, and faces for the
// mentors and sources behind the content.
import { cn } from '@hub/ui-kit';
import {
  CodeXml,
  ShieldCheck,
  Sparkles,
  Swords,
  Terminal,
  BrainCircuit,
  type LucideIcon,
} from 'lucide-react';
import { useState, type CSSProperties } from 'react';
import { useCurriculum } from './queries';

export interface AreaLook {
  color: string;
  icon: LucideIcon;
}

// OKLCH, mid lightness so the same hue reads in light and dark mode.
const areas: Record<string, AreaLook> = {
  foundations: { color: 'oklch(0.72 0.14 88)', icon: Terminal },
  // Orange, not red: a red Done button would read as destructive.
  offense: { color: 'oklch(0.68 0.18 48)', icon: Swords },
  'ai-security': { color: 'oklch(0.6 0.2 300)', icon: BrainCircuit },
  defense: { color: 'oklch(0.64 0.14 158)', icon: ShieldCheck },
  'ai-swe': { color: 'oklch(0.6 0.17 250)', icon: CodeXml },
};

export function areaLook(area: string | undefined): AreaLook {
  return areas[area ?? ''] ?? { color: 'oklch(0.6 0.17 250)', icon: Sparkles };
}

/** Style that sets --tc (the track colour) for the children's utilities. */
export function tint(area: string | undefined): CSSProperties {
  return { '--tc': areaLook(area).color } as CSSProperties;
}

/** A rounded tile with the area's icon. */
export function AreaIcon({
  area,
  size = 'md',
}: {
  area?: string;
  size?: 'sm' | 'md' | 'lg';
}) {
  const { icon: Icon } = areaLook(area);
  return (
    <span
      style={tint(area)}
      className={cn(
        'grid shrink-0 place-items-center rounded-xl bg-(--tc)/15 text-(--tc)',
        size === 'sm' && 'size-7 rounded-lg',
        size === 'md' && 'size-10',
        size === 'lg' && 'size-12',
      )}
    >
      <Icon
        className={
          size === 'sm' ? 'size-4' : size === 'lg' ? 'size-6' : 'size-5'
        }
      />
    </span>
  );
}

/** The skill area of a track (for its colour). */
export function useTrackArea(track: string): string | undefined {
  return useCurriculum().data?.tracks.find((t) => t.id === track)?.area;
}

/** id → name and picture for mentors and gap fillers. */
export function usePeople(): Map<string, { name: string; face?: string }> {
  const cur = useCurriculum().data;
  const m = new Map<string, { name: string; face?: string }>();
  cur?.mentors.forEach((p) => m.set(p.id, { name: p.name, face: p.face }));
  cur?.gapFillers.forEach((p) => m.set(p.id, { name: p.name, face: p.face }));
  return m;
}

const sizes = {
  xs: 'size-5 text-[9px]',
  sm: 'size-7 text-[10px]',
  md: 'size-9 text-xs',
  lg: 'size-12 text-sm',
};

/** A mentor's or source's face; initials if there is no picture. */
export function Face({
  id,
  size = 'sm',
  className,
}: {
  id?: string;
  size?: keyof typeof sizes;
  className?: string;
}) {
  const person = usePeople().get(id ?? '');
  const [failed, setFailed] = useState(false);
  if (!id || !person) return null;
  const initials = person.name
    .split(/\s+/)
    .map((w) => w[0])
    .filter((c) => c && /[A-Za-z]/.test(c))
    .slice(0, 2)
    .join('')
    .toUpperCase();
  const cls = cn(
    'shrink-0 rounded-full ring-2 ring-background',
    sizes[size],
    className,
  );
  if (person.face && !failed)
    return (
      <img
        src={person.face}
        alt=""
        title={person.name}
        loading="lazy"
        className={cn(cls, 'bg-muted object-cover')}
        onError={() => setFailed(true)}
      />
    );
  return (
    <span
      title={person.name}
      className={cn(
        cls,
        'grid place-items-center bg-muted font-semibold text-muted-foreground',
      )}
    >
      {initials}
    </span>
  );
}

/** Overlapping faces, e.g. a track's mentors. */
export function Faces({
  ids,
  size = 'sm',
}: {
  ids: string[];
  size?: keyof typeof sizes;
}) {
  return (
    <span className="flex -space-x-2">
      {ids.map((id) => (
        <Face key={id} id={id} size={size} />
      ))}
    </span>
  );
}
