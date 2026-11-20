import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Button, cn } from './index';

describe('cn', () => {
  it('lets later Tailwind classes win', () => {
    expect(cn('px-2 text-sm', undefined, 'px-4')).toBe('text-sm px-4');
  });
});

describe('Button', () => {
  it('renders a button with variant classes', () => {
    render(<Button variant="outline">Go</Button>);
    const b = screen.getByRole('button', { name: 'Go' });
    expect(b.className).toContain('border');
    expect(b.dataset.slot).toBe('button');
  });

  it('renders its child element with asChild', () => {
    render(
      <Button asChild>
        <a href="/x">Link</a>
      </Button>,
    );
    const a = screen.getByRole('link', { name: 'Link' });
    expect(a.tagName).toBe('A');
    expect(a.className).toContain('bg-primary');
  });
});
