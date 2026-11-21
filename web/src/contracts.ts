// Stable public DTOs for hosts embedding or extending the management client.
// The full wire contract is api/admin/v1/openapi.yaml.
export interface Identity { platform: string; scope: string; id: string }
export interface Player { name: string; uuid?: string; identity_mode: 'online' | 'forwarded' | 'offline' }
export interface Binding { network_id: string; player: Player; owner?: Identity; created_at: string; updated_at: string }
export interface Document { text: string; revision: string }
export interface Grant { capabilities: string[]; networks: string[]; routes: string[]; nodes: string[] }
export interface RenderResult { resource_id: string; mime: 'image/png' | 'image/jpeg'; width: number; height: number; sha256: string; size: number; created_at: string; expires_at: string }
export interface ErrorResponse { error: { code: string; message: string } }

// Configuration editors use one revision shared with the YAML source editor.
export type NodeAction = 'chat.send' | 'player.kick' | 'command.execute' | 'inventory.read' | 'tps.read' | 'permission.check' | 'placeholder.resolve';
export interface BotConnectionConfig {
  id: string; type: string; enabled: boolean; endpoint: string; token: string;
  account_id: string; allow_insecure_remote: boolean;
}
export interface GameNodeConfig {
  id: string; network_id: string; server_id: string; token: string;
  identity_mode: Player['identity_mode']; session_authority: boolean; login_gate: boolean; actions: NodeAction[];
}
export interface Conversation extends Identity { kind: 'private' | 'group' | 'channel' | 'thread' }
export interface MessageRouteConfig {
  id: string; connection_id: string; conversation: Conversation; node_ids: string[];
  to_game: boolean; to_bot: boolean; commands: boolean; notifications: boolean;
  unbind_on_leave: boolean; bot_prefix: string; game_prefix: string;
}
export interface CommandConfig {
  name: string; node_id: string; template: string; arguments: string[]; roles: string[];
  require_binding: boolean; cooldown_seconds: number;
}
export interface TaskSchedule { kind: 'interval' | 'daily'; every_seconds?: number; at?: string; timezone?: string }
export interface TaskAction { kind: 'bot.message' | 'chat.send' | 'command.execute'; route_id?: string; node_id?: string; text: string }
export interface TaskConfig { id: string; name: string; enabled: boolean; schedule: TaskSchedule; action: TaskAction }
export interface BindingConfig {
  enabled: boolean; code_ttl_seconds: number; max_attempts: number;
  attempt_window_seconds: number; max_players_per_identity: number; kick_on_unbind: boolean;
}
export interface RenderingConfig {
  browser_path: string; browser_version: string; concurrency: number; queue_size: number; timeout_seconds: number;
  online_template: string; profile_template: string; inventory_template: string; performance_template: string;
}
export interface ConfigurationSections {
  connections: BotConnectionConfig[] | null; nodes: GameNodeConfig[] | null;
  routes: MessageRouteConfig[] | null; commands: CommandConfig[] | null;
  tasks: TaskConfig[]; binding: BindingConfig; rendering: RenderingConfig;
}
export interface Configuration extends ConfigurationSections {
  config_version: 1; webui: boolean; inventory_retention_days: number;
  http: { listen: string; admin_token: string; allowed_hosts: string[] | null; tls_cert?: string; tls_key?: string; allow_insecure_remote: boolean };
}
export interface SettingsDocument { revision: string; config: Configuration; restart_required: boolean }
export interface ConfigSectionPatch {
  connections?: BotConnectionConfig[]; nodes?: GameNodeConfig[]; routes?: MessageRouteConfig[]; commands?: CommandConfig[];
  tasks?: TaskConfig[]; binding?: Partial<BindingConfig>; rendering?: Partial<RenderingConfig>;
}
export interface NodeConnection {
  id: string; network_id: string; server_id: string; connected: boolean; boot_id?: string; platform?: string;
  capabilities: string[]; allowed_actions: NodeAction[]; since?: string;
}
export interface PlayerCounts { players: number; online: number; bound: number; whitelist_only: number; unlisted: number }
export interface NetworkSummary { id: string; identity_mode: Player['identity_mode']; nodes: number; connected: number; counts: PlayerCounts }
export interface Dashboard {
  generated_at: string; counts: PlayerCounts; networks: NetworkSummary[];
  nodes: number; connected_nodes: number; adapters: number; enabled_adapters: number; connected_adapters: number;
  routes: number; commands: number;
}

export interface TaskSnapshot {
  revision: string; paused: boolean; reason: string;
  items: { config: TaskConfig; next_at?: string; running: boolean }[];
}
export interface TaskRun {
  id: string; task_id: string; task_name: string; revision: string;
  action_kind: TaskAction['kind']; target_id: string; trigger: 'scheduled' | 'manual';
  scheduled_at: string; created_at: string; started_at?: string; finished_at?: string;
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'unknown' | 'skipped' | 'canceled';
  error_code: string; message: string;
}
