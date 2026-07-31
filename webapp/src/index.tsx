// Disappearing Messages — webapp plugin entry.
//
// The skeleton registers the plugin with a no-op initialize. The retention
// badge, channel-header button and slash-command UI land in V2.3 (MPMD-30).

interface PluginRegistry {
  // Registry methods are populated as the V2.3 UI is added.
  [key: string]: unknown;
}

interface ReduxStore {
  getState: () => unknown;
  dispatch: (action: unknown) => unknown;
}

class DisappearingMessagesPlugin {
  public async initialize(_registry: PluginRegistry, _store: ReduxStore): Promise<void> {
    // Skeleton: plugin registered, no UI yet.
  }
}

declare global {
  interface Window {
    registerPlugin(pluginId: string, plugin: unknown): void;
  }
}

// Mirrors plugin.json -> webapp/src/manifest.ts in the canonical template.
const PLUGIN_ID = 'com.github.naicoi92.disappearing-messages';

window.registerPlugin(PLUGIN_ID, new DisappearingMessagesPlugin());

export default DisappearingMessagesPlugin;
