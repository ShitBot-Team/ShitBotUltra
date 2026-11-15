/* Every metric below comes from a Minecraft adapter's tps.read response. */
(() => {
  "use strict";
  const data = ultra.context.data;
  const servers = Array.isArray(data.servers) ? data.servers : [];
  const available = servers.filter(server => server.available && typeof server.metrics?.tps === "number");
  const text = (id, value) => { const node = document.getElementById(id); if (node) node.textContent = String(value); };
  text("title", ultra.context.theme.title || "Minecraft 服务器性能");
  text("description", "游戏节点采集的 tick TPS 与 MSPT");
  text("serverName", `${servers.length} 台`);
  text("updateTime", new Date(data.updated_at || Date.now()).toLocaleTimeString(ultra.context.locale));
  text("metricCount", "MC TPS / MSPT");
  text("healthText", available.length ? `${available.length} 台可用` : "不可用");
  text("healthDetail", available.length === servers.length ? "Minecraft 节点已返回测量值" : "部分节点离线或不支持 TPS");
  const root = document.getElementById("serverList"); root.replaceChildren();
  for (const server of servers.slice(0, 8)) {
    const card = document.createElement("article"); card.className = "server-card";
    const header = document.createElement("header"); header.className = "server-header";
    const title = document.createElement("h2"); title.textContent = String(server.server_id || server.node_id);
    header.append(title); card.append(header);
    const metrics = server.metrics;
    const rows = server.available && metrics && typeof metrics.tps === "number" ? [
      ["Minecraft TPS", metrics.tps.toFixed(2)],
      ["MSPT", typeof metrics.mspt === "number" ? metrics.mspt.toFixed(2) : "不支持"],
      ["在线玩家", `${metrics.online_players} / ${metrics.max_players}`],
      ["游戏进程内存", metrics.used_memory_bytes ? `${(metrics.used_memory_bytes / 1048576).toFixed(0)} MiB` : "不支持"]
    ] : [["Minecraft TPS", `不可用（${server.error_code || "METRIC_UNAVAILABLE"}）`]];
    for (const [label, value] of rows) {
      const row = document.createElement("div"); row.className = "ultra-metric";
      const key = document.createElement("span"); key.textContent = label;
      const detail = document.createElement("strong"); detail.textContent = String(value);
      row.append(key, detail); card.append(row);
    }
    root.append(card);
  }
  if (servers.length > 8) { const note = document.createElement("p"); note.className = "ultra-overflow"; note.textContent = `另有 ${servers.length - 8} 个游戏节点未在本图展开`; root.append(note); }
  ultra.ready();
})();
