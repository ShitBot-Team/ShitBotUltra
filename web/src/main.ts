// Use the same authored Vue components as the embedded, build-free entry.
// This optional Vite entry does not maintain a second copy of the UI.
import { mount } from '../../internal/webui/assets/app.js';
import '../../internal/webui/assets/style.css';

mount();
