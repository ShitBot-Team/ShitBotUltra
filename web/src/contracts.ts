// Stable public DTOs for hosts embedding or extending the management client.
// The full wire contract is api/admin/v1/openapi.yaml.
export interface Identity { platform: string; scope: string; id: string }
export interface Player { name: string; uuid?: string; identity_mode: 'online' | 'forwarded' | 'offline' }
export interface Binding { network_id: string; player: Player; owner?: Identity; created_at: string; updated_at: string }
export interface Document { text: string; revision: string }
export interface Grant { capabilities: string[]; networks: string[]; routes: string[]; nodes: string[] }
export interface RenderResult { resource_id: string; mime: 'image/png' | 'image/jpeg'; width: number; height: number; sha256: string; size: number; created_at: string; expires_at: string }
export interface ErrorResponse { error: { code: string; message: string } }
