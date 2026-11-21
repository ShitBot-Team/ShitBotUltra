# 定时任务

本模块用于 Core 自身的定时消息与游戏动作，与本地插件的 tick 回调分别管理。配置在 `config.yaml` 的 `tasks` 数组，默认 `[]`；保存后重启宿主生效。分阶段补丁中，0005 提供配置、时点计算与数据库迁移，0006 接入执行与管理 API，0007 提供管理页面。

## 配置

每个任务只执行一个目标上的一个动作，不展开变量或调用宿主 Shell。最多 128 个任务，ID 不重复。禁用任务仍校验引用，防止启用时才发现配置失效。

```yaml
tasks:
  - id: daily-notice
    name: 每日晚间提醒
    enabled: false
    schedule:
      kind: daily
      at: '20:00'
      timezone: Asia/Shanghai
    action:
      kind: bot.message
      route_id: main-group
      text: 欢迎来到服务器，请遵守群规。
  - id: game-notice
    name: 游戏内定时广播
    enabled: false
    schedule:
      kind: interval
      every_seconds: 3600
    action:
      kind: chat.send
      node_id: survival
      text: 输入帮助命令查看服务器玩法。
```

示例引用 [examples/config.yaml](../examples/config.yaml) 中的路由与节点。启用 `bot.message` 要求该路由的连接也已启用；它是显式任务，不受路由的聊天转发或进退服通知开关控制。`chat.send` / `command.execute` 要求节点允许同名动作，执行时还会检查在线能力。命令正文不带开头 `/`，不会通过操作系统执行。

正文为 1–2000 个 UTF-8 字节，禁止控制字符（包括换行）；中文通常每字 3 字节。名称最多 128 字节。任务表单通过 `PATCH /api/v1/config/sections` 修改 `tasks`，与配置原文共享 ETag，可一次性同时修改节点或路由及其任务引用。

## 时间语义

- `interval`：60–604800 秒，以 Unix epoch 为固定基准对齐，重启不会重新计时；例如 3600 秒对应 UTC 整点。
- `daily`：显式 `HH:MM` 与 IANA 时区，不接受依赖宿主的 `Local`。Core 嵌入时区数据，不要求 Windows 或精简系统安装 zoneinfo。
- 首次启动只安排当前时间之后的时点，不补发停机期间的任务。夏令时跳过的本地时刻不执行；重复的本地时刻最多尝试一次，重启后仍由持久化日期槽去重。
- 同一任务的定时时点有持久化高水位；系统时钟回拨不会重放已尝试的时点。任务移除后保留高水位，复用 ID 不会清空它。

## 数据升级

数据库 schema 从 1 升至 2，新增任务游标与执行记录两张表及索引，不改动已有绑定 / 玩家表。迁移使用原有事务机制。旧 Core 会拒绝打开 schema 2；回退需同时恢复升级前的数据库备份，不能只换旧二进制或手改 `user_version`。首次启动新版前请保留停机后的一致性数据库副本及配置。

本轮按维护者要求仅做源码审查，未编译、运行测试或启动调度器。
