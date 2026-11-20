# @hub/ui-kit

Shared React components and design tokens for the hub shell and app UIs. Optional for apps.

```css
/* your app's main CSS */
@import 'tailwindcss';
@import '@hub/ui-kit/theme.css'; /* tokens, light/dark, and @source for these components */
```

```tsx
import { Button, Card, Switch } from '@hub/ui-kit';
```

- Components follow [shadcn/ui](https://ui.shadcn.com) conventions (Radix primitives + Tailwind v4 + `cva`), written by hand so imports stay relative inside this package.
- Dark mode: `class="dark"` on `<html>`.
- Add a component: copy the shadcn source into `src/components/`, switch its `@/lib/utils` import to `../lib/utils`, export it from `src/index.ts`.
