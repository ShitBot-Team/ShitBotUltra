/* Ultra data adapter; upstream layout attribution is in template.json. */
(() => {
  "use strict";
  const data = ultra.context.data;
  const players = Array.isArray(data.players) ? data.players : [];
  const text = (id, value) => { const node = document.getElementById(id); if (node) node.textContent = String(value); };
  text("title", ultra.context.theme.title || "服务器在线玩家");
  text("subtitle", data.network_id || "Minecraft Server");
  text("totalOnline", players.length);
  text("updateTime", new Date(data.updated_at || Date.now()).toLocaleTimeString(ultra.context.locale));
  text("statusText", players.length ? "有人在线" : "暂无玩家在线");
  const groups = new Map();
  for (const profile of players) {
    const server = String(profile.node_id || data.network_id || "Minecraft");
    if (!groups.has(server)) groups.set(server, []);
    groups.get(server).push(String(profile.player?.name || "未知玩家"));
  }
  text("serverCount", groups.size);
  const root = document.getElementById("serverList");
  root.replaceChildren();
  let shown = 0;
  for (const [server, names] of groups) {
    if (shown >= 30) break;
    const card = document.createElement("article"); card.className = "server-card";
    const header = document.createElement("header"); header.className = "server-header";
    const title = document.createElement("h2"); title.textContent = server;
    const count = document.createElement("span"); count.textContent = `${names.length} 人`;
    header.append(title, count); card.append(header);
    const grid = document.createElement("div"); grid.className = "player-grid";
    for (const name of names) {
      if (shown >= 30) break;
      const player = document.createElement("div"); player.className = "player";
      const label = document.createElement("strong"); label.textContent = name;
      player.append(label); grid.append(player); shown++;
    }
    card.append(grid); root.append(card);
  }
  if (!players.length || shown < players.length) {
    const note = document.createElement("div"); note.className = "empty";
    note.textContent = players.length ? `另有 ${players.length - shown} 名玩家未在本图展开` : "当前没有在线玩家";
    root.append(note);
  }
  ultra.ready();
})();
