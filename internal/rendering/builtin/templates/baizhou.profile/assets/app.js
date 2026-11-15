/* Ultra data adapter; upstream layout attribution is in template.json. */
(() => {
  "use strict";
  const data = ultra.context.data;
  const name = String(data.player?.name || "未知玩家");
  const text = (id, value) => { const node = document.getElementById(id); if (node) node.textContent = String(value); };
  text("title", name);
  text("subtitle", ultra.context.theme.subtitle || "Minecraft Player");
  text("description", data.online ? "正在游戏中" : "当前离线");
  text("playerName", name);
  document.querySelector('[data-option="showAvatar"]')?.remove();
  const seconds = Math.max(0, Number(data.total_seconds) || 0);
  const rows = [
    ["累计在线", `${Math.floor(seconds / 3600)} 小时 ${Math.floor(seconds % 3600 / 60)} 分钟`],
    ["当前状态", data.online ? "在线" : "离线"],
    ["网络", data.network_id || "—"],
    ["会话节点", data.node_id || "—"],
    ["最近在线", data.last_seen_at ? new Date(data.last_seen_at).toLocaleString(ultra.context.locale) : "—"],
    ["身份模式", data.player?.identity_mode || "—"]
  ];
  if (Array.isArray(data.statistics)) {
    for (const row of data.statistics.slice(0, 18)) rows.push([String(row.label), String(row.value)]);
  }
  text("statCount", `${rows.length} 项`);
  const grid = document.getElementById("statsGrid"); grid.replaceChildren();
  for (const [label, value] of rows) {
    const row = document.createElement("div"); row.className = "ultra-metric";
    const title = document.createElement("span"); title.textContent = label;
    const detail = document.createElement("strong"); detail.textContent = value;
    row.append(title, detail); grid.append(row);
  }
  ultra.ready();
})();
