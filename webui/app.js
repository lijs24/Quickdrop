const messages = {
  "zh-CN": {
    settings: "设置",
    refresh: "刷新",
    devices: "设备",
    groups: "群组",
    selectConversation: "选择一个会话",
    disconnected: "未连接",
    connected: "已连接",
    reconnecting: "重连中",
    noMessagesYet: "暂无消息",
    file: "文件",
    clear: "清除",
    send: "发送",
    close: "关闭",
    save: "保存",
    role: "角色",
    language: "语言",
    deviceID: "设备 ID",
    displayName: "显示名称",
    deviceToken: "设备令牌",
    hubURL: "Hub 地址",
    sseURL: "SSE 地址",
    agentListen: "Agent 监听",
    agentDataDir: "Agent 数据目录",
    downloadsDir: "下载目录",
    guiListen: "GUI 监听",
    useSshTunnel: "使用 SSH 隧道",
    startSshTunnel: "启动 SSH 隧道",
    sshHost: "SSH 主机",
    localPort: "本地端口",
    remoteHost: "远端主机",
    remotePort: "远端端口",
    messagePlaceholder: "输入消息",
    thisDevice: "本机",
    online: "在线",
    offline: "离线",
    saving: "正在保存...",
    savedRestart: "已保存。连接、身份、监听或隧道设置需要重启 QuickDrop 后生效。",
  },
  en: {
    settings: "Settings",
    refresh: "Refresh",
    devices: "Devices",
    groups: "Groups",
    selectConversation: "Select a conversation",
    disconnected: "Disconnected",
    connected: "Connected",
    reconnecting: "Reconnecting",
    noMessagesYet: "No messages yet",
    file: "File",
    clear: "Clear",
    send: "Send",
    close: "Close",
    save: "Save",
    role: "Role",
    language: "Language",
    deviceID: "Device ID",
    displayName: "Display name",
    deviceToken: "Device token",
    hubURL: "Hub URL",
    sseURL: "SSE URL",
    agentListen: "Agent listen",
    agentDataDir: "Agent data dir",
    downloadsDir: "Downloads dir",
    guiListen: "GUI listen",
    useSshTunnel: "Use SSH tunnel",
    startSshTunnel: "Start SSH tunnel",
    sshHost: "SSH host",
    localPort: "Local port",
    remoteHost: "Remote host",
    remotePort: "Remote port",
    messagePlaceholder: "Message",
    thisDevice: "This device",
    online: "online",
    offline: "offline",
    saving: "Saving...",
    savedRestart: "Saved. Restart QuickDrop to apply connection, identity, listen, or tunnel changes.",
  },
};

const state = {
  me: null,
  devices: [],
  groups: [],
  current: null,
  messages: [],
  settingsConfig: null,
  language: "zh-CN",
  connectionState: "disconnected",
};

const els = {
  deviceLabel: document.getElementById("deviceLabel"),
  refreshButton: document.getElementById("refreshButton"),
  settingsButton: document.getElementById("settingsButton"),
  devices: document.getElementById("devices"),
  groups: document.getElementById("groups"),
  conversationTitle: document.getElementById("conversationTitle"),
  conversationMeta: document.getElementById("conversationMeta"),
  connectionState: document.getElementById("connectionState"),
  messages: document.getElementById("messages"),
  composer: document.getElementById("composer"),
  textInput: document.getElementById("textInput"),
  fileInput: document.getElementById("fileInput"),
  clearFileButton: document.getElementById("clearFileButton"),
  settingsDialog: document.getElementById("settingsDialog"),
  settingsForm: document.getElementById("settingsForm"),
  closeSettingsButton: document.getElementById("closeSettingsButton"),
  settingsPath: document.getElementById("settingsPath"),
  settingsStatus: document.getElementById("settingsStatus"),
  settingRole: document.getElementById("settingRole"),
  settingLanguage: document.getElementById("settingLanguage"),
  settingDeviceId: document.getElementById("settingDeviceId"),
  settingDisplayName: document.getElementById("settingDisplayName"),
  settingToken: document.getElementById("settingToken"),
  settingHubBaseUrl: document.getElementById("settingHubBaseUrl"),
  settingSseUrl: document.getElementById("settingSseUrl"),
  settingAgentListen: document.getElementById("settingAgentListen"),
  settingAgentDataDir: document.getElementById("settingAgentDataDir"),
  settingDownloadsDir: document.getElementById("settingDownloadsDir"),
  settingGuiListen: document.getElementById("settingGuiListen"),
  settingUseSshTunnel: document.getElementById("settingUseSshTunnel"),
  settingSshEnabled: document.getElementById("settingSshEnabled"),
  settingSshHost: document.getElementById("settingSshHost"),
  settingLocalPort: document.getElementById("settingLocalPort"),
  settingRemoteHost: document.getElementById("settingRemoteHost"),
  settingRemotePort: document.getElementById("settingRemotePort"),
};

function tr(key) {
  return (messages[state.language] && messages[state.language][key]) || messages.en[key] || key;
}

function applyLanguage() {
  document.documentElement.lang = state.language;
  document.querySelectorAll("[data-i18n]").forEach((node) => {
    node.textContent = tr(node.dataset.i18n);
  });
  els.textInput.placeholder = tr("messagePlaceholder");
  updateConversationHeader();
  setConnectionState(state.connectionState);
  renderLists();
  renderMessages();
}

async function api(path, options = {}) {
  const headers = options.headers || {};
  if (options.body && !(options.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }
  const response = await fetch(path, { ...options, headers });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || response.statusText);
  }
  const contentType = response.headers.get("content-type") || "";
  return contentType.includes("application/json") ? response.json() : response.text();
}

async function loadConfig() {
  state.me = await api("/config");
  state.language = normalizedLanguage(state.me.language);
  applyLanguage();
  els.deviceLabel.textContent = `${state.me.display_name} (${state.me.device_id})`;
}

async function refreshLists() {
  const [devices, groups] = await Promise.all([
    api("/api/devices"),
    api("/api/groups"),
  ]);
  state.devices = devices.devices || [];
  state.groups = groups.groups || [];
  renderLists();
  if (!state.current) {
    const firstOther = state.devices.find((d) => d.id !== state.me.device_id);
    if (firstOther) {
      selectConversation("device", firstOther.id);
    } else if (state.groups[0]) {
      selectConversation("group", state.groups[0].id);
    }
  }
}

function renderLists() {
  if (!els.devices || !state.me) return;
  els.devices.innerHTML = "";
  for (const device of state.devices) {
    const item = document.createElement("div");
    const isLocal = device.id === state.me.device_id;
    item.className = `${itemClass("device", device.id)}${isLocal ? " localDevice" : ""}`;
    const badge = isLocal ? ` <span class="localBadge">${escapeHtml(tr("thisDevice"))}</span>` : "";
    item.innerHTML = `<strong>${escapeHtml(device.display_name || device.id)}${badge}</strong><span>${escapeHtml(device.id)} - ${device.online ? tr("online") : tr("offline")}</span>`;
    item.addEventListener("click", () => selectConversation("device", device.id));
    els.devices.appendChild(item);
  }

  els.groups.innerHTML = "";
  for (const group of state.groups) {
    const item = document.createElement("div");
    item.className = itemClass("group", group.id);
    item.innerHTML = `<strong>${escapeHtml(group.name || group.id)}</strong><span>${escapeHtml(group.members.join(", "))}</span>`;
    item.addEventListener("click", () => selectConversation("group", group.id));
    els.groups.appendChild(item);
  }
}

function itemClass(type, id) {
  return state.current && state.current.type === type && state.current.id === id ? "item active" : "item";
}

async function selectConversation(type, id) {
  state.current = { type, id };
  renderLists();
  updateConversationHeader();
  await loadMessages();
}

function updateConversationHeader() {
  if (!state.current) {
    els.conversationTitle.textContent = tr("selectConversation");
    els.conversationMeta.textContent = "";
    return;
  }
  const title = state.current.type === "device"
    ? state.devices.find((d) => d.id === state.current.id)?.display_name || state.current.id
    : state.groups.find((g) => g.id === state.current.id)?.name || state.current.id;
  els.conversationTitle.textContent = title;
  els.conversationMeta.textContent = `${state.current.type}:${state.current.id}`;
}

async function loadMessages() {
  if (!state.current) {
    renderMessages();
    return;
  }
  const conversationID = `${state.current.type}:${state.current.id}`;
  const data = await api(`/api/messages?conversation_id=${encodeURIComponent(conversationID)}`);
  state.messages = data.messages || [];
  renderMessages();
}

function renderMessages() {
  if (!els.messages) return;
  els.messages.innerHTML = "";
  if (!state.current) {
    els.messages.innerHTML = `<div class="empty">${escapeHtml(tr("selectConversation"))}</div>`;
    return;
  }
  if (state.messages.length === 0) {
    els.messages.innerHTML = `<div class="empty">${escapeHtml(tr("noMessagesYet"))}</div>`;
    return;
  }
  for (const env of state.messages) {
    const message = env.message;
    const div = document.createElement("article");
    div.className = message.sender_device_id === state.me.device_id ? "message own" : "message";
    const status = env.delivery && env.delivery.status ? env.delivery.status : "sent";
    const attachments = (env.attachments || []).map((att) => {
      const href = `/api/attachments/${encodeURIComponent(att.id)}/download`;
      const size = formatBytes(att.size_bytes);
      return `<a href="${href}">${escapeHtml(att.original_name)}</a><span>${size}</span>`;
    }).join("");
    div.innerHTML = `
      <div class="messageHeader">
        <span>${escapeHtml(message.sender_device_id)}</span>
        <span>${escapeHtml(formatTime(message.created_at))}</span>
        <span>${escapeHtml(status)}</span>
      </div>
      ${message.text ? `<div class="messageText">${escapeHtml(message.text)}</div>` : ""}
      ${attachments ? `<div class="attachments">${attachments}</div>` : ""}
    `;
    els.messages.appendChild(div);
  }
  els.messages.scrollTop = els.messages.scrollHeight;
}

async function sendCurrent(event) {
  event.preventDefault();
  if (!state.current) return;
  const text = els.textInput.value.trim();
  const file = els.fileInput.files[0];
  if (!text && !file) return;

  if (file) {
    const form = new FormData();
    form.set("metadata", JSON.stringify({
      target_type: state.current.type,
      target_id: state.current.id,
      text,
    }));
    form.append("files", file);
    await api("/api/messages/file", { method: "POST", body: form });
  } else {
    await api("/api/messages/text", {
      method: "POST",
      body: JSON.stringify({
        target_type: state.current.type,
        target_id: state.current.id,
        text,
      }),
    });
  }
  els.textInput.value = "";
  els.fileInput.value = "";
  resizeComposer();
  await loadMessages();
}

async function openSettings() {
  els.settingsStatus.textContent = "";
  const data = await api("/settings");
  state.settingsConfig = data.config || {};
  fillSettings(data);
  if (els.settingsDialog.showModal) {
    els.settingsDialog.showModal();
  } else {
    els.settingsDialog.setAttribute("open", "");
  }
}

function fillSettings(data) {
  const cfg = data.config || {};
  const device = cfg.device || {};
  const agent = cfg.agent || {};
  const hubClient = cfg.hub_client || {};
  const sshTunnel = cfg.ssh_tunnel || {};
  const gui = cfg.gui || {};

  els.settingsPath.textContent = data.config_path || "";
  els.settingRole.value = cfg.role || "agent";
  els.settingLanguage.value = normalizedLanguage(gui.language);
  els.settingDeviceId.value = device.id || "";
  els.settingDisplayName.value = device.display_name || "";
  els.settingToken.value = device.token || "";
  els.settingHubBaseUrl.value = hubClient.base_url || data.effective_base_url || "";
  els.settingSseUrl.value = hubClient.sse_url || "";
  els.settingAgentListen.value = agent.listen || "";
  els.settingAgentDataDir.value = agent.data_dir || "";
  els.settingDownloadsDir.value = agent.downloads_dir || "";
  els.settingGuiListen.value = gui.listen || "";
  els.settingUseSshTunnel.checked = Boolean(hubClient.use_ssh_tunnel);
  els.settingSshEnabled.checked = Boolean(sshTunnel.enabled);
  els.settingSshHost.value = sshTunnel.ssh_host || "";
  els.settingLocalPort.value = sshTunnel.local_port || "";
  els.settingRemoteHost.value = sshTunnel.remote_host || "";
  els.settingRemotePort.value = sshTunnel.remote_port || "";
}

async function saveSettings(event) {
  event.preventDefault();
  const previous = state.settingsConfig || {};
  const nextLanguage = normalizedLanguage(els.settingLanguage.value);
  const next = {
    ...previous,
    role: els.settingRole.value,
    device: {
      ...(previous.device || {}),
      id: els.settingDeviceId.value.trim(),
      display_name: els.settingDisplayName.value.trim(),
      token: els.settingToken.value,
    },
    agent: {
      ...(previous.agent || {}),
      listen: els.settingAgentListen.value.trim(),
      data_dir: els.settingAgentDataDir.value.trim(),
      downloads_dir: els.settingDownloadsDir.value.trim(),
    },
    hub_client: {
      ...(previous.hub_client || {}),
      base_url: els.settingHubBaseUrl.value.trim(),
      sse_url: els.settingSseUrl.value.trim(),
      use_ssh_tunnel: els.settingUseSshTunnel.checked,
    },
    ssh_tunnel: {
      ...(previous.ssh_tunnel || {}),
      enabled: els.settingSshEnabled.checked,
      ssh_host: els.settingSshHost.value.trim(),
      local_port: numberOrZero(els.settingLocalPort.value),
      remote_host: els.settingRemoteHost.value.trim(),
      remote_port: numberOrZero(els.settingRemotePort.value),
    },
    gui: {
      ...(previous.gui || {}),
      listen: els.settingGuiListen.value.trim(),
      language: nextLanguage,
    },
  };

  els.settingsStatus.textContent = tr("saving");
  const saved = await api("/settings", {
    method: "POST",
    body: JSON.stringify(next),
  });
  state.settingsConfig = saved.config || next;
  state.language = nextLanguage;
  applyLanguage();
  fillSettings(saved);
  els.settingsStatus.textContent = tr("savedRestart");
}

function closeSettings() {
  if (els.settingsDialog.close) {
    els.settingsDialog.close();
  } else {
    els.settingsDialog.removeAttribute("open");
  }
}

function numberOrZero(value) {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function resizeComposer() {
  els.textInput.style.height = "auto";
  els.textInput.style.height = `${Math.min(els.textInput.scrollHeight, 180)}px`;
}

function normalizedLanguage(value) {
  return value === "en" ? "en" : "zh-CN";
}

function setConnectionState(nextState) {
  state.connectionState = nextState;
  els.connectionState.textContent = tr(nextState);
  els.connectionState.classList.toggle("connected", nextState === "connected");
}

function connectEvents() {
  const events = new EventSource("/api/events");
  events.onopen = () => {
    setConnectionState("connected");
  };
  events.onerror = () => {
    setConnectionState("reconnecting");
  };
  events.addEventListener("message", async () => {
    await refreshLists();
    await loadMessages();
  });
}

function formatTime(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(state.language === "zh-CN" ? "zh-CN" : undefined);
}

function formatBytes(value) {
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

async function boot() {
  try {
    applyLanguage();
    await loadConfig();
    await refreshLists();
    resizeComposer();
    connectEvents();
  } catch (err) {
    els.messages.innerHTML = `<div class="empty error">${escapeHtml(err.message)}</div>`;
  }
}

els.refreshButton.addEventListener("click", async () => {
  await refreshLists();
  await loadMessages();
});
els.composer.addEventListener("submit", sendCurrent);
els.textInput.addEventListener("input", resizeComposer);
els.textInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
    event.preventDefault();
    els.composer.requestSubmit();
  }
});
els.clearFileButton.addEventListener("click", () => {
  els.fileInput.value = "";
});
els.settingsButton.addEventListener("click", async () => {
  try {
    await openSettings();
  } catch (err) {
    els.messages.innerHTML = `<div class="empty error">${escapeHtml(err.message)}</div>`;
  }
});
els.settingLanguage.addEventListener("change", () => {
  state.language = normalizedLanguage(els.settingLanguage.value);
  applyLanguage();
});
els.closeSettingsButton.addEventListener("click", closeSettings);
els.settingsForm.addEventListener("submit", async (event) => {
  try {
    await saveSettings(event);
  } catch (err) {
    event.preventDefault();
    els.settingsStatus.textContent = err.message;
  }
});
boot();
