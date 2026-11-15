# Bridge v1 接入语义

公开 Go 类型在 `api/bridge/v1/types.go`，JSON 形状在 `envelope.schema.json`。Schema 不能代替节点鉴权、权限、身份、期限和快照新鲜度检查。本文用于独立 Java MC API / Minecraft 端接入，未声称已交付这些客户端。

## 连接与恢复

1. 游戏进程连接 `/bridge/v1` WebSocket，携带 `Authorization: Bearer <node token>` 和 `X-SBU-Node-ID: <configured node id>`，不发送 Origin。不启用消息压缩。一个节点同时只允许一个会话，重复连接返回 409。
2. 10 秒内发送 `kind=control,type=hello,sequence=0`。信封和 hello 中的节点 / boot ID 必须一致，network/server ID 必须匹配 Core 配置。游戏进程每次启动生成新的 boot ID，同一进程重连保留它。
3. Core 返回 `welcome`：协议版本、会话 ID、服务器时间、心跳间隔 20 秒、消息上限 3 MiB、持久化事件游标和 `snapshot_required`。每个方向有自己的序号，不能把 Core 的信封 sequence 当作游戏端游标。
4. 后续游戏端信封序号在当前连接中严格递增，范围 1 至 `2^53-1`。重连的新事件必须高于 `resume_sequence`。权威节点每次重连都需要先发一个新鲜的 `players.snapshot`，其 sequence 高于游标；不要先重放旧的加入 / 离开增量来覆盖当前快照。
5. Core 用 WebSocket ping 检测连接，10 秒无有效回应即断开；不是自定义 JSON 心跳。断开时关闭活动会话，重连快照重新建立状态，累计时长不计入停机时间。

事件处理成功并保存游标后，Core 发 `ack`，其中 payload.sequence 是已处理的游戏端事件游标。重复的已确认事件不会重新处理。ack 不证明外部机器人消息已送达，也不证明游戏动作已完成。聊天转发采用有界、最多一次尝试，不能据 ack 宣称可靠消息队列语义。

事件被拒绝时发 `event.rejected` 后断开。权威快照缺失时发 `snapshot.required` 后断开。请求队列上限 64，满时请求返回 BUSY，事件满时断开以便重新建立状态。

## 当前消息集合

| 方向 / kind | type | payload / 行为 |
| --- | --- | --- |
| 游戏 → Core / request | `login.authorize` | `{player}`；返回 allowed、可选拒绝原因、验证码与期限 |
| 游戏 → Core / request | `binding.get` | `{name}`；返回 player、whitelisted、bound，不返回社交账号凭据 |
| 游戏 → Core / event | `players.snapshot` | `{players:[...]}`，最多 10000 人，仅会话权威节点 |
| 游戏 → Core / event | `player.joined`、`player.left` | `{player}`，仅会话权威节点 |
| 游戏 → Core / event | `inventory.snapshot` | 玩家、采集时间、format_version=1、JSON 对象 data；数据最多 2 MiB |
| 游戏 → Core / event | `chat.message` | 玩家、文本、可选 origin_id；有 origin_id 的回声不再次转发 |
| 游戏 → Core / event | `server.ready` | 不读取业务 payload；每个 boot ID 最多一次通知 |
| Core → 游戏 / request | `chat.send` | `{text,origin_id}`；游戏端转发产生的聊天必须保留来源以抑制回环 |
| Core → 游戏 / request | `player.kick` | `{player,reason}` |
| Core → 游戏 / request | `command.execute` | `{command,actor_id}`，游戏控制台命令 |
| Core → 游戏 / request | `inventory.read` | `{player}`；返回 InventorySnapshot |
| Core → 游戏 / request | `tps.read` | `{}`；返回 ServerPerformance，无法测量时 TPS 为 null |
| Core → 游戏 / request | `permission.check`、`placeholder.resolve` | Core 透传经授权的 JSON；平台专属 payload / 结果需在对应 MC API 中约定，目前不伪造统一实现 |

所有玩家数据须符合网络 identity_mode。在线 / 转发模式的会话和背包数据必须携带 UUID。聊天正文最长 16000 UTF-8 字节，超过 30 秒的旧聊天不转发，未来时间偏差超过 30 秒会被拒绝。背包保留期限按配置处理，不用旧快照覆盖新快照。

`tps.read` 的 measured_at、tps、online_players、max_players 必须存在；tps_5m、tps_15m、mspt 和内存信息可选。Core 展示不超过一分钟的游戏采样，代理或不支持的平台显示不可用，不用 Core 吞吐量代替 TPS。

## 请求期限与副作用

request 必须有 request_id、deadline 和 timeout_ms（1–30000）。接收端以本地期限控制执行，考虑最多 30 秒时钟偏差；响应使用相同 request_id 和 type。Core 下发游戏动作的预算最多 15 秒；每节点最多 64 个待回复动作。

Core 给游戏动作设置 idempotency_key。游戏客户端必须在有界窗口内去重，并在适合的平台线程 / 区域线程执行动作。发送后超时、断开或响应类型不匹配时返回 `RESULT_UNKNOWN`，不能自动重试、不能宣称未执行。重启后的去重丢失也不能把结果改写为确定失败。客户端对 `login.authorize` 拒绝或连接不可用的处理属于游戏平台接入逻辑，应明确告知玩家。

成功响应包含 payload；失败响应包含 `{error:{code,message}}`。管理 API 的 `/nodes/{id}/actions` 同样遵守节点允许动作与握手能力的交集。管理 API 超时不意味着已经下发的动作被撤销。
