const messages = {
  "zh-CN": {
    settings: "设置",
    refresh: "刷新",
    deviceAdmin: "设备管理",
    newDevice: "新增设备",
    deleteDevice: "删除设备",
    generateToken: "生成",
    groupMemberships: "群组",
    savedDevice: "设备已保存",
    deletedDevice: "设备已删除",
    confirmDeleteDevice: "确定删除这个设备？",
    tokenRequired: "新设备需要令牌",
    monitor: "监控",
    hubPing: "Hub 延迟",
    hubTime: "Hub 时间",
    onlineDevices: "在线设备",
    pendingDeliveries: "待投递",
    sseConnections: "SSE 连接",
    lastSeen: "最后活跃",
    neverSeen: "从未连接",
    update: "更新",
    closeApp: "关闭应用",
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
    deviceColor: "设备颜色",
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
    checkingUpdate: "正在检查更新...",
    updateStarted: "更新程序已启动，QuickDrop 将关闭并应用新版本。",
    alreadyUpdated: "已经是最新版本。",
    noNewerRelease: "没有更新的正式版本。",
  },
  en: {
    settings: "Settings",
    refresh: "Refresh",
    deviceAdmin: "Devices",
    newDevice: "New device",
    deleteDevice: "Delete",
    generateToken: "Generate",
    groupMemberships: "Groups",
    savedDevice: "Device saved",
    deletedDevice: "Device deleted",
    confirmDeleteDevice: "Delete this device?",
    tokenRequired: "New devices require a token",
    monitor: "Monitor",
    hubPing: "Hub ping",
    hubTime: "Hub time",
    onlineDevices: "Online devices",
    pendingDeliveries: "Pending deliveries",
    sseConnections: "SSE connections",
    lastSeen: "Last seen",
    neverSeen: "Never seen",
    update: "Update",
    closeApp: "Close app",
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
    deviceColor: "Device color",
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
    checkingUpdate: "Checking for updates...",
    updateStarted: "Updater started. QuickDrop will close and apply the new version.",
    alreadyUpdated: "Already up to date.",
    noNewerRelease: "No newer release is available.",
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
  appMode: false,
  monitorTimer: null,
  deviceAdminSelected: null,
  deviceAdminNew: false,
};

const els = {
  deviceLabel: document.getElementById("deviceLabel"),
  refreshButton: document.getElementById("refreshButton"),
  deviceAdminButton: document.getElementById("deviceAdminButton"),
  monitorButton: document.getElementById("monitorButton"),
  updateButton: document.getElementById("updateButton"),
  closeAppButton: document.getElementById("closeAppButton"),
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
  settingColor: document.getElementById("settingColor"),
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
  deviceAdminDialog: document.getElementById("deviceAdminDialog"),
  closeDeviceAdminButton: document.getElementById("closeDeviceAdminButton"),
  deviceAdminStatus: document.getElementById("deviceAdminStatus"),
  deviceAdminList: document.getElementById("deviceAdminList"),
  newDeviceButton: document.getElementById("newDeviceButton"),
  deviceAdminForm: document.getElementById("deviceAdminForm"),
  adminDeviceId: document.getElementById("adminDeviceId"),
  adminDisplayName: document.getElementById("adminDisplayName"),
  adminDeviceColor: document.getElementById("adminDeviceColor"),
  adminDeviceToken: document.getElementById("adminDeviceToken"),
  generateTokenButton: document.getElementById("generateTokenButton"),
  adminGroupChecks: document.getElementById("adminGroupChecks"),
  deleteDeviceButton: document.getElementById("deleteDeviceButton"),
  monitorDialog: document.getElementById("monitorDialog"),
  closeMonitorButton: document.getElementById("closeMonitorButton"),
  monitorMeta: document.getElementById("monitorMeta"),
  hubPingValue: document.getElementById("hubPingValue"),
  onlineDevicesValue: document.getElementById("onlineDevicesValue"),
  pendingDeliveriesValue: document.getElementById("pendingDeliveriesValue"),
  monitorDevices: document.getElementById("monitorDevices"),
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
  if (els.deviceAdminDialog.open) {
    renderDeviceAdmin();
  }
}

async function api(path, options = {}) {
  const headers = options.headers || {};
  if (options.body && !(options.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }
  const response = await fetch(path, { ...options, headers });
  if (!response.ok) {
    const text = await response.text();
    try {
      const payload = JSON.parse(text);
      throw new Error(payload.error || text || response.statusText);
    } catch (err) {
      if (err instanceof SyntaxError) {
        throw new Error(text || response.statusText);
      }
      throw err;
    }
  }
  const contentType = response.headers.get("content-type") || "";
  return contentType.includes("application/json") ? response.json() : response.text();
}

async function loadConfig() {
  state.me = await api("/config");
  state.language = normalizedLanguage(state.me.language);
  state.appMode = state.me.app_mode === true || state.me.app_mode === "true";
  applyLanguage();
  syncAppControls();
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
    const color = colorForDevice(device.id);
    item.style.setProperty("--device-color", color);
    const badge = isLocal ? ` <span class="localBadge">${escapeHtml(tr("thisDevice"))}</span>` : "";
    item.innerHTML = `
      <div class="deviceNameRow">
        <span class="deviceColorDot"></span>
        <strong>${escapeHtml(device.display_name || device.id)}${badge}</strong>
      </div>
      <span>${escapeHtml(device.id)} - ${device.online ? tr("online") : tr("offline")}</span>
    `;
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
    const sender = deviceByID(message.sender_device_id);
    const senderColor = colorForDevice(message.sender_device_id);
    div.style.setProperty("--sender-color", senderColor);
    const status = env.delivery && env.delivery.status ? env.delivery.status : "sent";
    const attachments = (env.attachments || []).map((att) => {
      const href = `/api/attachments/${encodeURIComponent(att.id)}/download`;
      const size = formatBytes(att.size_bytes);
      return `<a href="${href}">${escapeHtml(att.original_name)}</a><span>${size}</span>`;
    }).join("");
    div.innerHTML = `
      <div class="messageHeader">
        <span class="messageSender"><span class="senderDot"></span>${escapeHtml(sender?.display_name || message.sender_device_id)}</span>
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
  els.settingColor.value = normalizeColor(device.color || colorForID(device.id || "device"));
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
      color: normalizeColor(els.settingColor.value),
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
  await refreshLists();
  renderMessages();
  els.settingsStatus.textContent = tr("savedRestart");
}

function closeSettings() {
  if (els.settingsDialog.close) {
    els.settingsDialog.close();
  } else {
    els.settingsDialog.removeAttribute("open");
  }
}

async function openDeviceAdmin() {
  els.deviceAdminStatus.textContent = "";
  await refreshLists();
  if (!state.deviceAdminSelected && state.devices[0]) {
    state.deviceAdminSelected = state.devices[0].id;
  }
  state.deviceAdminNew = false;
  renderDeviceAdmin();
  if (els.deviceAdminDialog.showModal) {
    els.deviceAdminDialog.showModal();
  } else {
    els.deviceAdminDialog.setAttribute("open", "");
  }
}

function closeDeviceAdmin() {
  if (els.deviceAdminDialog.close) {
    els.deviceAdminDialog.close();
  } else {
    els.deviceAdminDialog.removeAttribute("open");
  }
}

function renderDeviceAdmin() {
  els.deviceAdminList.innerHTML = "";
  for (const device of state.devices) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = `deviceAdminItem${!state.deviceAdminNew && state.deviceAdminSelected === device.id ? " active" : ""}`;
    item.style.setProperty("--device-color", normalizeColor(device.color || colorForID(device.id)));
    item.innerHTML = `
      <span class="deviceColorDot"></span>
      <span>
        <strong>${escapeHtml(device.display_name || device.id)}</strong>
        <small>${escapeHtml(device.id)} - ${device.online ? tr("online") : tr("offline")}</small>
      </span>
    `;
    item.addEventListener("click", () => {
      state.deviceAdminNew = false;
      state.deviceAdminSelected = device.id;
      els.deviceAdminStatus.textContent = "";
      renderDeviceAdmin();
    });
    els.deviceAdminList.appendChild(item);
  }
  fillDeviceAdminForm();
}

function fillDeviceAdminForm() {
  const device = state.deviceAdminNew ? null : deviceByID(state.deviceAdminSelected);
  const deviceID = device?.id || "";
  els.adminDeviceId.value = deviceID;
  els.adminDeviceId.disabled = Boolean(device);
  els.adminDisplayName.value = device?.display_name || "";
  els.adminDeviceColor.value = normalizeColor(device?.color || colorForID(deviceID || "new-device"));
  els.adminDeviceToken.value = "";
  els.adminDeviceToken.placeholder = device ? "" : tr("tokenRequired");
  els.deleteDeviceButton.disabled = !device || device.id === state.me.device_id;

  const selectedGroups = new Set(device ? groupIDsForDevice(device.id) : defaultNewDeviceGroups());
  els.adminGroupChecks.innerHTML = "";
  for (const group of state.groups) {
    const label = document.createElement("label");
    label.className = "adminGroupCheck";
    const checked = selectedGroups.has(group.id) ? "checked" : "";
    label.innerHTML = `
      <input type="checkbox" value="${escapeHtml(group.id)}" ${checked}>
      <span>${escapeHtml(group.name || group.id)}</span>
    `;
    els.adminGroupChecks.appendChild(label);
  }
}

function startNewDevice() {
  state.deviceAdminNew = true;
  state.deviceAdminSelected = "";
  els.deviceAdminStatus.textContent = "";
  renderDeviceAdmin();
  els.adminDeviceToken.value = generateToken();
  els.adminDeviceId.focus();
}

async function saveAdminDevice(event) {
  event.preventDefault();
  const existing = !state.deviceAdminNew && deviceByID(state.deviceAdminSelected);
  const token = els.adminDeviceToken.value.trim();
  if (!existing && !token) {
    els.deviceAdminStatus.textContent = tr("tokenRequired");
    return;
  }
  const body = {
    id: (existing?.id || els.adminDeviceId.value).trim(),
    display_name: els.adminDisplayName.value.trim(),
    color: normalizeColor(els.adminDeviceColor.value),
    token,
    group_ids: selectedAdminGroupIDs(),
  };
  const saved = await api("/api/devices", {
    method: "POST",
    body: JSON.stringify(body),
  });
  state.deviceAdminNew = false;
  state.deviceAdminSelected = saved.id || body.id;
  await refreshLists();
  renderDeviceAdmin();
  els.deviceAdminStatus.textContent = tr("savedDevice");
}

async function deleteAdminDevice() {
  const device = deviceByID(state.deviceAdminSelected);
  if (!device) return;
  if (!window.confirm(tr("confirmDeleteDevice"))) return;
  await api(`/api/devices/${encodeURIComponent(device.id)}`, { method: "DELETE" });
  state.deviceAdminSelected = "";
  await refreshLists();
  state.deviceAdminSelected = state.devices[0]?.id || "";
  renderDeviceAdmin();
  els.deviceAdminStatus.textContent = tr("deletedDevice");
}

function selectedAdminGroupIDs() {
  return Array.from(els.adminGroupChecks.querySelectorAll("input[type='checkbox']:checked")).map((input) => input.value);
}

function groupIDsForDevice(deviceID) {
  return state.groups
    .filter((group) => Array.isArray(group.members) && group.members.includes(deviceID))
    .map((group) => group.id);
}

function defaultNewDeviceGroups() {
  return state.groups.some((group) => group.id === "all") ? ["all"] : [];
}

function generateToken() {
  const bytes = new Uint8Array(18);
  if (window.crypto && window.crypto.getRandomValues) {
    window.crypto.getRandomValues(bytes);
  } else {
    for (let i = 0; i < bytes.length; i += 1) {
      bytes[i] = Math.floor(Math.random() * 256);
    }
  }
  return `qd_${Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

async function openMonitor() {
  if (els.monitorDialog.showModal) {
    els.monitorDialog.showModal();
  } else {
    els.monitorDialog.setAttribute("open", "");
  }
  await refreshMonitor();
  stopMonitorTimer();
  state.monitorTimer = window.setInterval(() => {
    refreshMonitor().catch(() => {});
  }, 3000);
}

function closeMonitor() {
  stopMonitorTimer();
  if (els.monitorDialog.close) {
    els.monitorDialog.close();
  } else {
    els.monitorDialog.removeAttribute("open");
  }
}

function stopMonitorTimer() {
  if (state.monitorTimer) {
    window.clearInterval(state.monitorTimer);
    state.monitorTimer = null;
  }
}

async function refreshMonitor() {
  const pingStart = performance.now();
  await api("/api/health");
  const pingMs = Math.max(0, Math.round(performance.now() - pingStart));
  const data = await api("/api/monitor");
  renderMonitor(data, pingMs);
}

function renderMonitor(data, pingMs) {
  const devices = data.devices || [];
  const onlineCount = devices.filter((device) => device.online).length;
  const pendingCount = devices.reduce((sum, device) => sum + Number(device.pending_deliveries || 0), 0);
  els.hubPingValue.textContent = `${pingMs} ms`;
  els.onlineDevicesValue.textContent = `${onlineCount} / ${devices.length}`;
  els.pendingDeliveriesValue.textContent = String(pendingCount);
  els.monitorMeta.textContent = data.hub_time ? `${tr("hubTime")}: ${formatTime(data.hub_time)}` : "";
  els.monitorDevices.innerHTML = "";
  for (const device of devices) {
    const row = document.createElement("div");
    row.className = "monitorDevice";
    row.style.setProperty("--device-color", normalizeColor(device.color || colorForID(device.id)));
    const lastSeen = device.last_seen_at ? relativeTime(device.last_seen_at) : tr("neverSeen");
    row.innerHTML = `
      <div class="monitorDeviceMain">
        <span class="deviceColorDot"></span>
        <div>
          <strong>${escapeHtml(device.display_name || device.id)}</strong>
          <span>${escapeHtml(device.id)}</span>
        </div>
      </div>
      <div class="monitorBadges">
        <span class="${device.online ? "statusBadge onlineBadge" : "statusBadge offlineBadge"}">${device.online ? tr("online") : tr("offline")}</span>
        <span class="statusBadge">${escapeHtml(tr("sseConnections"))}: ${Number(device.sse_connections || 0)}</span>
        <span class="statusBadge">${escapeHtml(tr("pendingDeliveries"))}: ${Number(device.pending_deliveries || 0)}</span>
        <span class="statusBadge">${escapeHtml(tr("lastSeen"))}: ${escapeHtml(lastSeen)}</span>
      </div>
    `;
    els.monitorDevices.appendChild(row);
  }
}

function deviceByID(id) {
  return state.devices.find((device) => device.id === id);
}

function colorForDevice(id) {
  const device = deviceByID(id);
  return normalizeColor(device?.color || colorForID(id));
}

function colorForID(id) {
  const palette = ["#2563eb", "#7c3aed", "#0f766e", "#d97706", "#dc2626", "#0891b2", "#65a30d", "#be185d"];
  let hash = 0;
  for (const ch of String(id || "device")) {
    hash = (hash * 31 + ch.charCodeAt(0)) >>> 0;
  }
  return palette[hash % palette.length];
}

function normalizeColor(value) {
  const color = String(value || "").trim();
  return /^#[0-9a-fA-F]{6}$/.test(color) ? color.toLowerCase() : "#0f766e";
}

function syncAppControls() {
  for (const button of [els.updateButton, els.closeAppButton]) {
    if (!button) continue;
    button.hidden = !state.appMode;
  }
}

function startAppHeartbeat() {
  if (!state.appMode) return;
  const beat = () => {
    fetch("/app/heartbeat", { method: "POST", keepalive: true }).catch(() => {});
  };
  beat();
  window.setInterval(beat, 2000);
}

async function closeApp() {
  if (!state.appMode) return;
  await api("/app/shutdown", { method: "POST" });
  window.setTimeout(() => {
    window.close();
  }, 250);
}

async function updateApp() {
  if (!state.appMode) return;
  const originalText = els.updateButton.textContent;
  els.updateButton.disabled = true;
  els.updateButton.textContent = tr("checkingUpdate");
  try {
    const result = await api("/app/update", { method: "POST" });
    if (result.already_current || result.AlreadyCurrent) {
      const currentVersion = result.current_version || result.CurrentVersion;
      const targetVersion = result.target_version || result.TargetVersion;
      els.updateButton.textContent = currentVersion && targetVersion && currentVersion !== targetVersion
        ? `${tr("noNewerRelease")} ${currentVersion} -> ${targetVersion}`
        : tr("alreadyUpdated");
      window.setTimeout(() => {
        els.updateButton.textContent = originalText;
        els.updateButton.disabled = false;
      }, 2500);
      return;
    }
    els.updateButton.textContent = tr("updateStarted");
  } catch (err) {
    els.updateButton.textContent = originalText;
    els.updateButton.disabled = false;
    els.messages.innerHTML = `<div class="empty error">${escapeHtml(err.message)}</div>`;
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
    refreshLists().then(loadMessages).catch(() => {});
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

function relativeTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const seconds = Math.max(0, Math.round((Date.now() - date.getTime()) / 1000));
  if (seconds < 45) return state.language === "zh-CN" ? "刚刚" : "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return state.language === "zh-CN" ? `${minutes} 分钟前` : `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return state.language === "zh-CN" ? `${hours} 小时前` : `${hours}h ago`;
  return formatTime(value);
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
    startAppHeartbeat();
  } catch (err) {
    els.messages.innerHTML = `<div class="empty error">${escapeHtml(err.message)}</div>`;
  }
}

els.refreshButton.addEventListener("click", async () => {
  await refreshLists();
  await loadMessages();
});
els.updateButton.addEventListener("click", updateApp);
els.closeAppButton.addEventListener("click", closeApp);
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
els.deviceAdminButton.addEventListener("click", async () => {
  try {
    await openDeviceAdmin();
  } catch (err) {
    els.messages.innerHTML = `<div class="empty error">${escapeHtml(err.message)}</div>`;
  }
});
els.monitorButton.addEventListener("click", async () => {
  try {
    await openMonitor();
  } catch (err) {
    closeMonitor();
    els.messages.innerHTML = `<div class="empty error">${escapeHtml(err.message)}</div>`;
  }
});
els.settingLanguage.addEventListener("change", () => {
  state.language = normalizedLanguage(els.settingLanguage.value);
  applyLanguage();
});
els.closeSettingsButton.addEventListener("click", closeSettings);
els.closeDeviceAdminButton.addEventListener("click", closeDeviceAdmin);
els.newDeviceButton.addEventListener("click", startNewDevice);
els.generateTokenButton.addEventListener("click", () => {
  els.adminDeviceToken.value = generateToken();
});
els.deleteDeviceButton.addEventListener("click", async () => {
  try {
    await deleteAdminDevice();
  } catch (err) {
    els.deviceAdminStatus.textContent = err.message;
  }
});
els.closeMonitorButton.addEventListener("click", closeMonitor);
els.monitorDialog.addEventListener("close", stopMonitorTimer);
els.monitorDialog.addEventListener("cancel", stopMonitorTimer);
els.deviceAdminForm.addEventListener("submit", async (event) => {
  try {
    await saveAdminDevice(event);
  } catch (err) {
    event.preventDefault();
    els.deviceAdminStatus.textContent = err.message;
  }
});
els.settingsForm.addEventListener("submit", async (event) => {
  try {
    await saveSettings(event);
  } catch (err) {
    event.preventDefault();
    els.settingsStatus.textContent = err.message;
  }
});
boot();
