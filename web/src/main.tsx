import React from 'react';
import { createRoot } from 'react-dom/client';
import { HashRouter } from 'react-router-dom';
import App from './App';
import { initPwa } from './pwa';

import './styles/tokens.css';
import './styles/base.css';
import './styles/layout.css';
import './styles/grid.css';
import './styles/lightbox.css';
import './styles/player.css';
import './styles/forms.css';

createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <HashRouter>
      <App />
    </HashRouter>
  </React.StrictMode>,
);

initPwa();
