import React from 'react';
import { createRoot } from 'react-dom/client';

// Bundle the brand fonts locally (offline-safe) instead of fetching Google
// Fonts — the desktop app is the "surface that may ship external assets"
// the web ux-spec anticipates.
import '@fontsource/figtree/400.css';
import '@fontsource/figtree/500.css';
import '@fontsource/figtree/600.css';
import '@fontsource/figtree/700.css';
import '@fontsource/figtree/800.css';
import '@fontsource/ibm-plex-mono/400.css';
import '@fontsource/ibm-plex-mono/500.css';
import '@fontsource/ibm-plex-mono/600.css';

// Design system, verbatim from the handoff (source of truth), then shell base.
import './styles/tokens.css';
import './styles/ui.css';
import './styles/app.css';
import './styles/desktop.css';
import './styles/shell.css';

import App from './App.jsx';

createRoot(document.getElementById('root')).render(<App />);
