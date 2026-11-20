import type { CategoryId } from './api';

export const categories: { id: CategoryId; name: string; color: string }[] = [
  { id: 'swe', name: 'SWE', color: 'oklch(0.62 0.16 255)' },
  { id: 'security', name: 'Security', color: 'oklch(0.63 0.2 25)' },
  { id: 'ai', name: 'AI', color: 'oklch(0.62 0.18 305)' },
];

export const categoryColor = (id: CategoryId) =>
  categories.find((c) => c.id === id)?.color ?? 'gray';
export const categoryName = (id: CategoryId) =>
  categories.find((c) => c.id === id)?.name ?? id;
