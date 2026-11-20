import { connect, syncThemeClass } from '@hub/app-sdk';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './app';
import './styles.css';

// One bridge per page. Outside the hub it still works (theme follows the OS).
const hub = connect();
syncThemeClass(hub);

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <App hub={hub} />
  </StrictMode>,
);
