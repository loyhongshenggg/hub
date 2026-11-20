import { Button, Tooltip, TooltipContent, TooltipTrigger } from '@hub/ui-kit';
import { Monitor, Moon, Sun } from 'lucide-react';
import { type Theme, useUI } from '../state/ui';

const order: Theme[] = ['system', 'light', 'dark'];
const icons = { system: Monitor, light: Sun, dark: Moon };
const labels = {
  system: 'Theme: system',
  light: 'Theme: light',
  dark: 'Theme: dark',
};

export function ThemeToggle() {
  const { theme, setTheme } = useUI();
  const Icon = icons[theme];
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={labels[theme]}
          onClick={() =>
            setTheme(order[(order.indexOf(theme) + 1) % order.length])
          }
          className="text-muted-foreground"
        >
          <Icon />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="right">{labels[theme]}</TooltipContent>
    </Tooltip>
  );
}
