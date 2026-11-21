import { createApp, h, onBeforeUnmount } from './shared/vue.js';
import { createSession } from './app/session.js';
import Login from './app/login.js';
import Shell from './app/shell.js';

export function mount() {
  return createApp({
    setup() {
      const context = createSession();
      onBeforeUnmount(context.logout);
      return () => h(context.state.authenticated ? Shell : Login, { context });
    }
  }).mount('#app');
}
