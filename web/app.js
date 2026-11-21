import { errorLabel, setLanguage, toolLabel, tr } from "./i18n.js";
import { button, field, icon, iconButton, input, markdown, node, saved } from "./ui.js";

const call = (method, ...args) => window.mygo.call("Desktop." + method, ...args);
const app = document.getElementById("app");
const toast = document.getElementById("toast");
const dialog = document.getElementById("dialog");
const state = {
  folders: [], sessions: new Map(), expanded: new Set(), options: new Map(), threads: new Map(), drafts: new Map(),
  directory: "", session: "", panel: "", sidebar: saved.get("sidebar") !== "hidden",
  language: "zh-CN", version: "", engineError: "", connected: false, active: new Set(), submitting: new Set(),
  filePath: ".", previewPath: "", editing: false,
};

const sidebar = node("aside", "sidebar");
const sideTop = node("div", "sidebar-top");
const sideBody = node("nav", "sidebar-body");
const sideFooter = node("div", "sidebar-footer");
sidebar.append(sideTop, sideBody, sideFooter);

const center = node("main", "center");
const titlebar = node("header", "titlebar");
const stage = node("section", "stage draft");
const scroll = node("div");
scroll.id = "scroll";
const transcript = node("div", "transcript");
scroll.append(transcript);
const hero = node("div", "hero");
const dock = node("div", "dock");
const dockInner = node("div", "dock-inner");
const gates = node("div");
const footer = node("footer", "footer");
dock.append(dockInner);
stage.append(scroll, hero, dock);
center.append(titlebar, stage, footer);

const right = node("aside", "right");
const rightTop = node("header", "right-top");
const rightBody = node("div", "panel-content");
right.append(rightTop, rightBody);
const rail = node("nav", "rail");
app.append(sidebar, center, right, rail);

const load = (method, ...args) => call(method, ...args).catch((error) => {
  report(error);
  return undefined;
});

let toastTimer;
function report(error) {
  toast.textContent = errorLabel(typeof error === "string" ? error : error?.message || String(error));
  toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { toast.hidden = true; }, 5500);
}

function activeKey(directory = state.directory, id = state.session) {
  return JSON.stringify([directory, id]);
}

function threadFor(directory = state.directory, id = state.session) {
  const key = activeKey(directory, id);
  if (!state.threads.has(key)) {
    state.threads.set(key, {
      records: new Map(), live: new Map(), previous: "", loaded: false, pending: false,
      permissions: [], forms: [], messagesVersion: 0, renderedVersion: -1, gateSignature: "",
    });
  }
  return state.threads.get(key);
}

function draftFor(directory = state.directory, id = state.session) {
  const key = activeKey(directory, id);
  if (!state.drafts.has(key)) {
    state.drafts.set(key, { text: saved.get("draft:" + key), files: [], skills: [], model: null, mode: "" });
  }
  return state.drafts.get(key);
}

const currentFolder = () => state.folders.find((folder) => folder.directory === state.directory);
const currentSession = () => (state.sessions.get(state.directory) || []).find((session) => session.id === state.session)
  || threadFor().session;

function activate(directory, id = "") {
  if (directory !== state.directory) {
    state.filePath = ".";
    state.previewPath = "";
    state.editing = false;
  }
  state.directory = directory;
  state.session = id;
  if (directory) state.expanded.add(directory);
  saved.set("lastWorkspace", directory);
  saved.set("lastSession:" + directory, id);
  renderedKey = "";
  renderShell();
  if (directory) {
    void loadOptions(directory);
    void fetchSessions(directory);
    if (id) {
      void fetchThread(directory, id);
      void refreshActive(directory);
    }
  }
  void renderPanel();
}

function openSession(directory, session) {
  threadFor(directory, session.id).session = session;
  activate(directory, session.id);
}

async function addFolder() {
  const folder = await load("PickWorkspace");
  if (!folder) return;
  state.folders = await call("Workspaces") || [];
  activate(folder.directory);
  composerInput.focus();
}

function newTask(directory = state.directory) {
  if (!directory) return addFolder();
  activate(directory);
  composerInput.focus();
}

async function loadOptions(directory, force = false) {
  if (state.options.has(directory) && !force) return;
  try {
    const result = await call("Options", directory);
    state.options.set(directory, result);
    if (state.directory === directory) renderComposerMeta();
  } catch (error) {
    if (state.directory === directory && !state.engineError) report(error);
  }
}

async function fetchSessions(directory) {
  try {
    const sessions = await call("Sessions", directory, false, "");
    state.sessions.set(directory, sessions || []);
    renderSidebar();
    if (state.directory === directory) renderHeader();
  } catch (error) {
    if (!state.engineError) console.warn(error);
  }
}

function renderSidebar() {
  sidebar.classList.toggle("collapsed", !state.sidebar);
  sideTop.replaceChildren();
  const toggle = iconButton(state.sidebar ? tr("Hide sidebar") : tr("Show sidebar"), "panel-left", toggleSidebar);
  toggle.classList.add("sidebar-toggle");
  sideTop.append(toggle);
  sideBody.replaceChildren();
  const search = button(tr("Search sessions"), showSearch, { glyph: "search", className: "side-row search-row" });
  search.disabled = state.folders.length === 0;
  search.setAttribute("aria-label", tr("Search sessions"));

  search.append(node("span", "key", "⇧⌘F"));
  sideBody.append(search);
  for (const folder of state.folders) {
    const group = node("div", "group");
    const header = node("div", "group-header");
    const expanded = state.expanded.has(folder.directory);
    const row = button(folder.name, () => {
      if (state.directory !== folder.directory) {
        activate(folder.directory);
      } else if (state.expanded.delete(folder.directory)) {
        renderSidebar();
      } else {
        state.expanded.add(folder.directory);
        renderSidebar();
        void fetchSessions(folder.directory);
      }
    }, { className: "side-row folder-row" });
    row.firstElementChild.classList.add("truncate");
    row.prepend(node("span", "orb"));
    row.append(node("span", "spacer"), icon(expanded ? "chevron-down" : "chevron-right"));
    row.setAttribute("aria-expanded", String(expanded));
    const more = iconButton(tr("Workspace actions"), "ellipsis", () => workspaceActions(folder));
    more.classList.add("row-more");
    header.append(row, more);
    group.append(header);
    if (expanded) {
      const list = node("div", "group-list");
      const draft = button(tr("New task"), () => newTask(folder.directory), { glyph: "plus", className: "side-row new-task" });
      if (state.directory === folder.directory && !state.session) draft.classList.add("selected");
      list.append(draft);
      for (const session of state.sessions.get(folder.directory) || []) {
        const item = node("div", "session-row");
        const pick = button(session.title || tr("Untitled session"), () => openSession(folder.directory, session), { className: "side-row" });
        pick.firstElementChild.classList.add("truncate");
        if (session.id === state.session && folder.directory === state.directory) pick.classList.add("selected");
        if (state.active.has(session.id)) pick.prepend(node("span", "dot running"));
        const actions = iconButton(tr("Session actions"), "ellipsis", () => sessionActions(folder.directory, session));
        actions.classList.add("row-more");
        item.append(pick, actions);
        list.append(item);
      }
      group.append(list);
    }
    sideBody.append(group);
  }
  sideFooter.replaceChildren(button(tr("Add workspace"), addFolder, { glyph: "plus", className: "side-row" }));
}

function toggleSidebar() {
  state.sidebar = !state.sidebar;
  saved.set("sidebar", state.sidebar ? "shown" : "hidden");
  renderSidebar();
  renderHeader();
}

function renderHeader() {
  const title = state.session ? currentSession()?.title || tr("Session") : currentFolder() ? tr("New task") : "MyGo Work";
  titlebar.replaceChildren();
  if (!state.sidebar) titlebar.append(iconButton(tr("Show sidebar"), "panel-left", toggleSidebar));
  titlebar.append(node("h1", "", title));
  const folder = currentFolder();
  if (folder) titlebar.append(node("span", "context", folder.name));
  titlebar.append(node("span", "spacer"));
  if (state.session) {
    titlebar.append(iconButton(tr("Session actions"), "ellipsis", () => sessionActions(state.directory, currentSession() || { id: state.session, title })));
  }
  titlebar.append(iconButton(tr("New task"), "plus", () => newTask()));
}

function renderFooter() {
  const running = state.active.has(state.session) || state.submitting.has(activeKey());
  const label = state.engineError ? tr("OpenCode unavailable") : !state.version ? tr("Starting OpenCode…") :
    !state.connected ? tr("Reconnecting…") : running ? tr("Task in progress") : tr("Ready for new tasks");
  const status = button(label, showSettings);
  status.prepend(node("span", "dot " + (state.engineError || !state.connected ? "" : running ? "running" : "ready")));
  footer.replaceChildren(status, node("span", "spacer"),
    button(tr("Docs"), () => load("OpenLink", "https://opencode.ai/docs/"), { glyph: "book-open" }),
    iconButton(tr("Settings"), "settings", showSettings));
}

const composer = node("form", "composer");
const chips = node("div", "chips");
const composerInput = node("textarea");
composerInput.rows = 2;
const controls = node("div", "composer-row");
composer.append(chips, composerInput, controls);

const suggestions = () => [
  [tr("Summarize a document"), tr("Summarize the main points of the documents I attach. Include any decisions and action items.")],
  [tr("Clean up a spreadsheet"), tr("Review my CSV for inconsistent values and missing information. Suggest a cleanup plan and show a preview.")],
  [tr("Draft a document"), tr("Help me turn my notes into a one-page project brief. Ask for any missing details before drafting.")],
  [tr("Explore this workspace"), tr("Read the workspace and summarize its structure, key files, and where to start.")],
];

function renderShell() {
  composerInput.placeholder = tr("Describe your task…");
  composerInput.setAttribute("aria-label", tr("Describe your task"));
  renderSidebar();
  renderHeader();
  renderFooter();
  const draft = !state.session;
  stage.classList.toggle("draft", draft);
  hero.hidden = !draft;
  dock.hidden = draft;
  hero.replaceChildren();
  if (draft && !state.directory) {
    const welcome = node("div", "welcome");
    welcome.append(node("span", "orb"), node("h2", "", tr("Start with a workspace")),
      node("p", "", tr("Choose a folder to give your tasks a place to work.")),
      button(tr("Choose folder"), addFolder, { glyph: "folder-open", className: "primary" }));
    hero.append(welcome);
  } else if (draft) {
    hero.append(node("h2", "", tr("What do you need done?")), composer);
    const list = node("div", "suggestions");
    for (const [title, prompt] of suggestions()) {
      const suggestion = button(title, () => {
        composerInput.value = prompt;
        composerInput.dispatchEvent(new Event("input"));
        composerInput.focus();
      }, { className: "suggestion" });
      suggestion.append(icon("arrow-right"));
      list.append(suggestion);
    }
    hero.append(list);
  } else {
    dockInner.replaceChildren(gates, composer);
  }
  composerInput.value = draftFor().text;
  resizeInput();
  renderComposerMeta();
  renderThread(true);
  renderGates(true);
  renderRail();
}

function resizeInput() {
  composerInput.rows = Math.min(6, Math.max(2, composerInput.value.split("\n").length));
}

composerInput.addEventListener("input", () => {
  draftFor().text = composerInput.value;
  saved.set("draft:" + activeKey(), composerInput.value);
  resizeInput();
  updateSubmit();
});
composerInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    if (!submitButton?.disabled && !state.active.has(state.session)) void send();
  }
});
composer.addEventListener("submit", (event) => { event.preventDefault(); void send(); });

const selection = (draft = draftFor()) => {
  const options = state.options.get(state.directory);
  const session = state.session && currentSession();
  const model = draft.model || (session?.model?.id ? session.model : options?.defaultModel) || {};
  const mode = draft.mode || session?.mode || (options?.modes || []).find((item) => item.id === "build")?.id || "";
  return { model, mode, options };
};

let submitButton;
function updateSubmit() {
  if (!submitButton) return;
  if (state.active.has(state.session)) {
    submitButton.disabled = false;
    return;
  }
  const draft = draftFor();
  submitButton.disabled = !state.directory || state.submitting.has(activeKey()) ||
    (!draft.text.trim() && !draft.files.length && !draft.skills.length);
}

function renderComposerMeta() {
  if (!state.directory) return;
  const draft = draftFor();
  chips.replaceChildren();
  for (const item of [...draft.files, ...draft.skills]) {
    const isSkill = !item.uri;
    chips.append(button(item.name, () => {
      const list = isSkill ? "skills" : "files";
      draft[list] = draft[list].filter((other) => other !== item);
      renderComposerMeta();
    }, { glyph: "x", className: "chip", title: tr("Remove {name}", { name: item.name }) }));
  }
  chips.hidden = !chips.children.length;
  const { model, mode, options } = selection(draft);
  const modelInfo = options?.models?.find((item) => item.id === model.id && item.provider === model.provider);
  const modelLabel = modelInfo?.name || model.id || tr("Model");
  const modeLabel = modeName(options?.modes?.find((item) => item.id === mode)) || mode || tr("Agent");
  const modeButton = button(modeLabel, chooseMode);
  const modelButton = button(modelLabel, chooseModel, { className: "pill model" });
  modeButton.append(icon("chevron-down"));
  modelButton.append(icon("chevron-down"));
  controls.replaceChildren(iconButton(tr("Attach files"), "paperclip", pickAttachments),
    iconButton(tr("Skills and connections"), "plug", () => setPanel(state.panel === "tools" ? "" : "tools")), modeButton, modelButton);
  if (modelInfo?.variants?.length) {
    const variant = button(tr(model.variant || "Default"), chooseVariant, { className: "pill variant" });
    variant.append(icon("chevron-down"));
    controls.append(variant);
  }
  const running = state.active.has(state.session);
  submitButton = button(running ? tr("Stop") : state.submitting.has(activeKey()) ? tr("Sending…") : tr("Run task"),
    running ? interrupt : send, { glyph: running ? "square" : "arrow-up", className: "primary" });
  controls.append(node("span", "spacer"), submitButton);
  updateSubmit();
}

async function pickAttachments() {
  const draft = draftFor();
  const items = await load("PickAttachments", state.directory);
  if (!items) return;
  for (const item of items) {
    if (!draft.files.some((existing) => existing.uri === item.uri)) draft.files.push(item);
  }
  renderComposerMeta();
}

async function send() {
  const directory = state.directory;
  let id = state.session;
  const originalId = id;
  const originalKey = activeKey(directory, id);
  const draft = draftFor(directory, id);
  if (!directory || state.active.has(id) || state.submitting.has(originalKey) ||
      (!draft.text.trim() && !draft.files.length && !draft.skills.length)) return;
  const { model, mode } = selection(draft);
  const payload = { text: draft.text.trim(), files: draft.files, skills: draft.skills.map((item) => item.id), model, mode };
  state.submitting.add(originalKey);
  renderComposerMeta();
  try {
    if (!id) {
      const session = await call("Create", directory, payload);
      id = session.id;
      threadFor(directory, id).session = session;
      state.sessions.set(directory, [session, ...(state.sessions.get(directory) || [])]);
    }
    const prompt = originalId ? payload : { ...payload, model: {}, mode: "" };
    await call("Prompt", directory, id, prompt);
    if (draft.text.trim() === payload.text) {
      draft.text = "";
      saved.set("draft:" + originalKey, "");
    }
    draft.files = draft.files.filter((item) => !payload.files.includes(item));
    draft.skills = draft.skills.filter((item) => !payload.skills.includes(item.id));
    state.active.add(id);
    state.submitting.delete(originalKey);
    if (state.directory === directory && state.session === originalId) {
      if (!originalId) openSession(directory, threadFor(directory, id).session);
      else {
        composerInput.value = draft.text;
        renderComposerMeta();
        renderFooter();
      }
      composerInput.focus({ preventScroll: true });
    }
    void fetchThread(directory, id);
    void refreshActive(directory);
  } catch (error) {
    if (!originalId && id) {
      threadFor(directory, id).session = (state.sessions.get(directory) || []).find((session) => session.id === id);
      if (state.directory === directory && !state.session) {
        const retry = draftFor(directory, id);
        Object.assign(retry, draft);
        saved.set("draft:" + activeKey(directory, id), retry.text);
        openSession(directory, threadFor(directory, id).session);
      }
    }
    report(error);
  } finally {
    state.submitting.delete(originalKey);
    renderComposerMeta();
  }
}

async function interrupt() {
  const { directory, session } = state;
  await load("Interrupt", directory, session);
  void refreshActive(directory);
  void fetchThread(directory, session);
}

let renderedKey = "";
let liveFrame = 0;

async function refreshActive(directory) {
  try {
    const result = new Set(await call("Active", directory) || []);
    const changed = [...result].some((id) => !state.active.has(id))
      || [...state.active].some((id) => !result.has(id));
    state.active = result;
    if (changed) {
      renderSidebar();
      renderComposerMeta();
      renderFooter();
      const thread = threadFor();
      thread.messagesVersion++;
      renderThread();
    }
  } catch (error) {
    if (!state.engineError) console.warn(error);
  }
}

async function fetchThread(directory, id) {
  if (!id) return;
  const thread = threadFor(directory, id);
  if (thread.loading) {
    thread.pending = true;
    return;
  }
  thread.loading = true;
  try {
    const [history, interactions] = await Promise.all([
      call("History", directory, id, ""),
      call("Interactions", directory, id),
    ]);
    let changed = false;
    for (const message of history.messages || []) {
      if (JSON.stringify(thread.records.get(message.id)) !== JSON.stringify(message)) {
        thread.records.set(message.id, message);
        changed = true;
      }
      if (message.complete && thread.live.delete(message.id)) changed = true;
    }
    if (!thread.loaded) thread.previous = history.previous;
    thread.loaded = true;
    thread.error = "";
    thread.permissions = interactions.permissions || [];
    thread.forms = interactions.forms || [];
    if (changed) thread.messagesVersion++;
  } catch (error) {
    if (!thread.loaded) {
      thread.error = error?.message || String(error);
      thread.messagesVersion++;
    }
  } finally {
    thread.loading = false;
    if (directory === state.directory && id === state.session) {
      renderThread();
      renderGates();
    }
    if (thread.pending) {
      thread.pending = false;
      scheduleThread(directory, id);
    }
  }
}

async function earlier() {
  const { directory, session } = state;
  const thread = threadFor();
  const result = await load("History", directory, session, thread.previous);
  if (!result) return;
  for (const message of result.messages || []) thread.records.set(message.id, message);
  thread.previous = result.previous;
  thread.messagesVersion++;
  if (session !== state.session || directory !== state.directory) return;
  const height = scroll.scrollHeight;
  const top = scroll.scrollTop;
  renderThread();
  scroll.scrollTop = top + scroll.scrollHeight - height;
}

const threadTimers = new Map();
function scheduleThread(directory, id) {
  const key = activeKey(directory, id);
  if (threadTimers.has(key)) return;
  threadTimers.set(key, setTimeout(() => {
    threadTimers.delete(key);
    void fetchThread(directory, id);
  }, 160));
}

function displayMessage(message, key) {
  const element = node("article", "message " + message.role);
  element.dataset.messageId = message.id;
  if (message.role === "idle") {
    if (message.error) element.append(node("div", "callout destructive", message.error));
    else if (message.outcome === "interrupted" || message.outcome === "aborted") {
      element.append(node("div", "busy", tr("Task stopped")));
    }
    return element;
  }
  if (message.error) element.append(node("p", "destructive", message.error));
  let index = 0;
  for (const part of message.parts || []) {
    if (part.kind === "text") {
      element.append(message.role === "user" ? node("div", "", part.text) : markdown(part.text, openLink));
    } else if (part.kind === "reasoning" && part.text) {
      const details = node("details", "reasoning");
      details.dataset.detail = key + ":" + index;
      const summary = node("summary", "", tr("Thinking"));
      summary.prepend(icon("chevron-right"));
      details.append(summary, markdown(part.text, openLink));
      element.append(details);
    } else if (part.kind === "tool") {
      const details = node("details", "tool");
      details.dataset.detail = key + ":" + index;
      const summary = node("summary");
      const label = part.name ? toolLabel(part.name) : tr("Tool");
      const inputText = typeof part.input === "string" ? part.input : JSON.stringify(part.input || {}, null, 2);
      const outline = part.input?.description || part.input?.command || part.input?.filePath || part.input?.path;
      const toolIcon = part.name?.includes("bash") || part.name?.includes("shell") ? "terminal" : "pen-line";
      summary.append(icon(toolIcon), node("span", "truncate", label + (outline ? " · " + outline : "")),
        node("span", "spacer"), node("small", "", tr(part.status || "")), icon("chevron-right"));
      details.append(summary, node("pre", "", [inputText, part.output].filter(Boolean).join("\n\n")));
      element.append(details);
      const file = part.input?.filePath || part.input?.path;
      const writeTool = /edit|patch|write/.test(part.name);
      if (writeTool && part.status === "completed" && typeof file === "string") {
        const directory = state.directory;
        const relative = file.startsWith(directory + "/") ? file.slice(directory.length + 1) : file;
        if (!relative.startsWith("/") && !relative.split("/").includes("..")) {
          element.append(button(relative, () => previewFile(relative), { glyph: "file-text", className: "secondary" }));
        }
      }
    }
    index++;
  }
  for (const file of message.files || []) {
    const chip = node("span", "chip", file.name || tr("Attachment"));
    chip.prepend(icon("paperclip"));
    element.append(chip);
  }
  return element;
}

function openLink(address) { void load("OpenLink", address); }

function renderThread(force = false) {
  if (!state.session) {
    transcript.replaceChildren();
    return;
  }
  const thread = threadFor();
  const key = activeKey();
  if (!force && key === renderedKey && thread.renderedVersion === thread.messagesVersion) return;
  const switched = key !== renderedKey;
  const atBottom = switched || scroll.scrollHeight - scroll.scrollTop - scroll.clientHeight < 80;
  const opened = switched ? new Set() : new Set([...transcript.querySelectorAll("details[open]")].map((item) => item.dataset.detail));
  renderedKey = key;
  thread.renderedVersion = thread.messagesVersion;
  transcript.replaceChildren();
  if (thread.previous) transcript.append(button(tr("Load earlier messages"), earlier, { glyph: "chevron-up", className: "secondary" }));
  if (thread.error) transcript.append(node("p", "destructive", errorLabel(thread.error)));
  if (!thread.loaded && !thread.error) transcript.append(node("p", "busy", tr("Loading session…")));
  const ids = [...new Set([...thread.records.keys(), ...thread.live.keys()])].sort();
  for (const id of ids) {
    const stored = thread.records.get(id);
    const live = thread.live.get(id);
    let message = stored;
    if (live && !stored?.complete) {
      const parts = [...stored?.parts || []];
      for (const [ordinal, part] of [...live].sort((a, b) => a[0] - b[0])) {
        const existing = parts[ordinal];
        if (!existing || existing.kind === part.kind && part.text.length >= (existing.text?.length || 0)) parts[ordinal] = part;
      }
      message = { id, role: "assistant", ...stored, parts: parts.filter(Boolean) };
    }
    const element = displayMessage(message, id);
    if (element.children.length) transcript.append(element);
  }
  if (state.active.has(state.session) && !thread.permissions.length && !thread.forms.length) {
    const busy = node("div", "busy", tr("Working…"));
    busy.prepend(node("span", "dot running"));
    transcript.append(busy);
  }
  for (const item of transcript.querySelectorAll("details")) item.open = opened.has(item.dataset.detail);
  if (atBottom) scroll.scrollTop = scroll.scrollHeight;
}

function receipt(event) {
  const runtime = currentFolder()?.runtime || "opencode";
  if (event.runtime && event.runtime !== runtime) return;
  if (event.kind === "catalog") {
    if (state.panel === "tools" && (!event.directory || event.directory === state.directory)) void renderPanel();
    return;
  }
  if (event.kind === "connected" || event.kind === "disconnected") {
    state.connected = event.kind === "connected";
    renderFooter();
    if (state.connected) {
      if (!state.version || state.engineError) void checkVersion();
      for (const dir of state.expanded) void fetchSessions(dir);
      if (state.directory) void refreshActive(state.directory);
      if (state.session) scheduleThread(state.directory, state.session);
    }
    return;
  }
  const { directory, session } = state;
  if (event.kind === "delta" && session === event.sessionID) {
    const thread = threadFor();
    if (thread.records.get(event.messageID)?.complete) return;
    let parts = thread.live.get(event.messageID);
    if (!parts) {
      parts = new Map();
      thread.live.set(event.messageID, parts);
    }
    const ordinal = event.ordinal || 0;
    const text = (parts.get(ordinal)?.text || "") + event.delta;
    parts.set(ordinal, { kind: event.part, text });
    thread.messagesVersion++;
    if (!liveFrame) {
      liveFrame = requestAnimationFrame(() => {
        liveFrame = 0;
        renderThread();
      });
    }
  } else if (event.kind !== "delta" && session && (event.sessionID === session || !event.sessionID)) {
    scheduleThread(directory, session);
  }
  if (event.kind === "settled") {
    if (directory) void refreshActive(directory);
    for (const dir of state.expanded) void fetchSessions(dir);
    if (state.panel === "changes" || (state.panel === "files" && !state.editing)) void renderPanel();
  }
}

function renderGates(force = false) {
  if (!state.session) return;
  const { directory, session } = state;
  const thread = threadFor();
  const signature = JSON.stringify([thread.permissions, thread.forms, directory, session]);
  if (!force && thread.gateSignature === signature) return;
  thread.gateSignature = signature;
  gates.replaceChildren();
  gates.hidden = !thread.permissions.length && !thread.forms.length;
  for (const request of thread.permissions) {
    const item = node("section", "callout");
    const body = [request.message, ...request.resources || []].filter(Boolean).join("\n");
    const actions = node("div", "actions");
    item.append(node("strong", "", tr("Permission requested · {action}", { action: toolLabel(request.action) })), node("pre", "", body), actions);
    for (const [label, decision] of [[tr("Deny"), "reject"], [tr("Always allow"), "always"], [tr("Allow once"), "once"]]) {
      actions.append(button(label, async () => {
        for (const control of actions.querySelectorAll("button")) control.disabled = true;
        try {
          await call("Decide", directory, session, request.id, decision);
          await fetchThread(directory, session);
        } catch (error) {
          report(error);
          for (const control of actions.querySelectorAll("button")) control.disabled = false;
        }
      }, { className: decision === "once" ? "primary" : "secondary" }));
    }
    gates.append(item);
  }
  for (const form of thread.forms) gates.append(renderQuestion(directory, session, form));
}

let fieldsSerial = 0;
function questionInput(spec) {
  const values = spec.options || [];
  let element;
  if (spec.type === "boolean") {
    element = node("select");
    for (const [value, label] of [["", tr("Choose…")], ["true", tr("Yes")], ["false", tr("No")]]) element.add(new Option(label, value));
    element.value = spec.default === undefined ? "" : String(spec.default);
  } else if (spec.type === "multiselect" || (values.length && !spec.custom)) {
    element = node("select");
    element.multiple = spec.type === "multiselect";
    if (!element.multiple) element.add(new Option(tr("Choose…"), ""));
    for (const option of values) {
      const selected = element.multiple ? (spec.default || []).includes(option.value) : spec.default === option.value;
      element.add(new Option(option.label, option.value, selected, selected));
    }
    if (!element.multiple) element.value = spec.default || "";
  } else {
    const numeric = ["integer", "number"].includes(spec.type);
    element = input(numeric ? "number" : "text", spec.default ?? "", spec.description || "");
    if (numeric) element.step = spec.type === "integer" ? "1" : "any";
    if (values.length) {
      const list = node("datalist");
      list.id = "field-" + String(fieldsSerial++);
      for (const option of values) list.append(new Option(option.label, option.value));
      element.setAttribute("list", list.id);
      return { element, list };
    }
  }
  return { element };
}

function renderQuestion(directory, session, question) {
  const wrap = node("section", "callout");
  const form = node("form");
  const fields = new Map();
  form.append(node("strong", "", question.title || tr("Input requested")));
  for (const spec of question.fields || []) {
    if (spec.hidden || spec.type === "external") continue;
    const { element, list } = questionInput(spec);
    const wrapper = field(spec.title || spec.key, element);
    if (list) wrapper.append(list);
    form.append(wrapper);
    fields.set(spec.key, { element, spec, wrapper });
  }
  const valueOf = ({ element, spec }) => {
    if (element.multiple) return [...element.selectedOptions].map((option) => option.value);
    if (element.value === "") return undefined;
    if (spec.type === "integer" || spec.type === "number") return Number(element.value);
    if (spec.type === "boolean") return element.value === "true";
    return element.value;
  };
  const validate = () => {
    for (const { element, spec, wrapper } of fields.values()) {
      wrapper.hidden = !(spec.when || []).every(({ key, op, value }) => fields.has(key) && (op === "neq" ? valueOf(fields.get(key)) !== value : valueOf(fields.get(key)) === value));
      element.required = !!spec.required && !wrapper.hidden;
    }
  };
  form.addEventListener("change", validate);
  validate();
  const actions = node("div", "actions");
  const respond = async (answer) => {
    for (const control of actions.querySelectorAll("button")) control.disabled = true;
    try {
      await call("Answer", directory, session, question.id, answer);
      await fetchThread(directory, session);
    } catch (error) {
      report(error);
      for (const control of actions.querySelectorAll("button")) control.disabled = false;
    }
  };
  const submit = button(tr("Submit"), null, { className: "primary" });
  submit.type = "submit";
  actions.append(button(tr("Dismiss"), () => respond(null), { className: "secondary" }), submit);
  form.append(actions);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const answer = {};
    for (const [key, entry] of fields) {
      const value = valueOf(entry);
      if (value !== undefined && !entry.wrapper.hidden) answer[key] = value;
    }
    void respond(answer);
  });
  wrap.append(form);
  return wrap;
}

function showDialog(title) {
  const header = node("header", "dialog-head");
  header.append(node("h2", "", title), node("span", "spacer"), iconButton(tr("Close"), "x", () => dialog.close()));
  const content = node("div", "dialog-body");
  dialog.replaceChildren(header, content);
  if (!dialog.open) dialog.showModal();
  return content;
}

function promptForm(title, fields, action, submitLabel = tr("Save")) {
  const wrap = showDialog(title);
  const form = node("form");
  const error = node("p", "destructive");
  error.hidden = true;
  const actions = node("div", "actions");
  const submit = button(submitLabel, null, { className: "primary" });
  submit.type = "submit";
  actions.append(button(tr("Cancel"), () => dialog.close(), { className: "secondary" }), submit);
  form.append(...fields, error, actions);
  wrap.append(form);
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    submit.disabled = true;
    error.hidden = true;
    try {
      await action();
      if (wrap.isConnected) dialog.close();
    } catch (problem) {
      error.textContent = errorLabel(problem?.message || String(problem));
      error.hidden = false;
    } finally {
      submit.disabled = false;
    }
  });
  fields[0]?.querySelector("input, select, textarea")?.focus();
}

function confirmAction(title, message, action, label) {
  const wrap = showDialog(title);
  const actions = node("div", "actions");
  const confirm = button(label, async () => {
    confirm.disabled = true;
    try {
      await action();
      if (wrap.isConnected) dialog.close();
    } catch (error) {
      report(error);
      confirm.disabled = false;
    }
  }, { className: "secondary destructive" });
  actions.append(button(tr("Cancel"), () => dialog.close(), { className: "secondary" }), confirm);
  wrap.append(node("p", "", message), actions);
}

function picker(title, items, select, selected, placeholder = tr("Search…")) {
  const wrap = showDialog(title);
  const search = input("search", "", placeholder);
  search.className = "search-input";
  search.setAttribute("aria-label", placeholder);
  const list = node("div", "picker-list");
  const update = () => {
    const query = search.value.toLowerCase().trim();
    const options = items.filter((item) => [item.label, item.detail].some((value) => value?.toLowerCase().includes(query)));
    list.replaceChildren();
    for (const item of options) {
      const row = button(item.label, () => { dialog.close(); select(item); }, { className: "picker-item" });
      row.setAttribute("aria-selected", String(item.key === selected));
      if (item.detail) row.append(node("small", "", item.detail));
      list.append(row);
    }
    if (!options.length) list.append(node("p", "", tr("No results found.")));
  };
  search.addEventListener("input", update);
  search.addEventListener("keydown", (event) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      list.querySelector("button")?.focus();
    }
    if (event.key === "Enter") list.querySelector("button")?.click();
  });
  wrap.append(search, list);
  update();
  search.focus();
}

function modeName(item) {
  return item?.id === "build" ? tr("Build") : item?.id === "plan" ? tr("Plan") : item?.name;
}

async function chooseMode() {
  await loadOptions(state.directory);
  const { mode, options } = selection();
  const items = (options?.modes || []).map((item) => ({ key: item.id, label: modeName(item) }));
  picker(tr("Select agent"), items, (item) => { draftFor().mode = item.key; renderComposerMeta(); }, mode);
}

async function chooseModel() {
  await loadOptions(state.directory);
  const { model, options } = selection();
  const keyFor = (item) => item.provider + "/" + item.id;
  const values = (options?.models || []).map((item) => ({
    key: keyFor(item), label: item.name, detail: item.provider, model: { provider: item.provider, id: item.id },
  }));
  picker(tr("Select model"), values, (item) => { draftFor().model = item.model; renderComposerMeta(); }, keyFor(model), tr("Search models…"));
  const wrap = dialog.querySelector(".dialog-body");
  wrap?.append(button(tr("Connect a provider"), showProviders, { glyph: "plus", className: "pill" }));
}

function chooseVariant() {
  const { model, options } = selection();
  const found = options.models.find((item) => item.id === model.id && item.provider === model.provider);
  const variants = [{ key: "", label: tr("Default") }, ...(found?.variants || []).map((id) => ({ key: id, label: tr(id) }))];
  picker(tr("Model effort"), variants, (item) => {
    draftFor().model = { ...model, variant: item.key };
    renderComposerMeta();
  }, model.variant || "");
}

function showSearch() {
  if (!state.folders.length) return;
  const wrap = showDialog(tr("Search sessions"));
  const search = input("search", "", tr("Search by session title"));
  search.className = "search-input";
  search.setAttribute("aria-label", tr("Search by session title"));
  const archive = input("checkbox");
  const filter = node("label", "filter-line");
  filter.append(archive, document.createTextNode(tr("Archived sessions")));
  const list = node("div", "picker-list");
  let generation = 0;
  let timer;
  const update = async () => {
    const current = ++generation;
    list.replaceChildren(node("p", "", tr("Searching…")));
    try {
      const query = search.value.trim();
      const folders = await Promise.all(state.folders.map(async (folder) => ({
        folder, sessions: await call("Sessions", folder.directory, archive.checked, query),
      })));
      if (!wrap.isConnected || generation !== current) return;
      list.replaceChildren();
      for (const { folder, sessions } of folders) {
        for (const session of sessions || []) {
          const row = button(session.title || tr("Untitled session"), () => {
            dialog.close();
            openSession(folder.directory, session);
          }, { className: "picker-item" });
          row.append(node("small", "truncate", folder.name));
          list.append(row);
        }
      }
      if (!list.children.length) list.append(node("p", "", tr("No sessions found.")));
    } catch (error) {
      if (wrap.isConnected && generation === current) list.replaceChildren(node("p", "destructive", errorLabel(error.message)));
    }
  };
  search.addEventListener("input", () => {
    generation++;
    clearTimeout(timer);
    timer = setTimeout(update, 180);
  });
  archive.addEventListener("change", update);
  search.addEventListener("keydown", (event) => {
    if (event.key === "Enter") list.querySelector("button")?.click();
  });
  wrap.append(search, filter, list);
  search.focus();
  void update();
}

function workspaceActions(folder) {
  const content = showDialog(folder.name);
  content.append(node("p", "", folder.directory));
  const list = node("div", "picker-list");
  list.append(
    button(tr("Show in Finder"), () => { dialog.close(); void load("Reveal", folder.directory, "."); }, { glyph: "folder-open", className: "picker-item" }),
    button(tr("Rename workspace"), () => {
      const name = input("text", folder.name);
      name.required = true;
      promptForm(tr("Rename workspace"), [field(tr("Workspace name"), name)], async () => {
        await call("RenameWorkspace", folder.directory, name.value);
        state.folders = await call("Workspaces") || [];
        renderSidebar();
        renderHeader();
      });
    }, { glyph: "pen-line", className: "picker-item" }),
    button(tr("Remove from sidebar"), () => {
      confirmAction(tr("Remove workspace?"), tr("The folder and its sessions will be available if you add it again."), async () => {
        await call("RemoveWorkspace", folder.directory);
        state.folders = await call("Workspaces") || [];
        state.expanded.delete(folder.directory);
        if (state.directory === folder.directory) activate(state.folders[0]?.directory || "");
        else renderSidebar();
      }, tr("Remove workspace"));
    }, { glyph: "x", className: "picker-item destructive" }),
  );
  content.append(list);
}

function sessionActions(directory, session) {
  const wrap = showDialog(session.title || tr("Session"));
  const list = node("div", "picker-list");
  list.append(
    button(tr("Rename session"), () => {
      const name = input("text", session.title || "");
      name.required = true;
      promptForm(tr("Rename session"), [field(tr("Session title"), name)], async () => {
        const title = name.value.trim();
        if (!title) throw new Error(tr("Enter a session title."));
        await call("Update", directory, session.id, { title });
        const updated = { ...session, title };
        threadFor(directory, session.id).session = updated;
        state.sessions.set(directory, (state.sessions.get(directory) || []).map((item) => item.id === session.id ? updated : item));
        renderHeader();
        await fetchSessions(directory);
      });
    }, { glyph: "pen-line", className: "picker-item" }),
    button(session.archived ? tr("Restore session") : tr("Archive session"), async () => {
      try {
        await call("Update", directory, session.id, { archived: !session.archived });
        if (state.directory === directory && state.session === session.id) {
          if (!session.archived) activate(directory);
          else threadFor(directory, session.id).session = { ...session, archived: false };
        }
        void fetchSessions(directory);
        dialog.close();
      } catch (error) { report(error); }
    }, { glyph: session.archived ? "rotate-ccw" : "archive", className: "picker-item" }),
    button(tr("Delete session"), () => confirmAction(tr("Delete session?"), tr("This permanently removes the task and its conversation history."), async () => {
      if (state.active.has(session.id)) await call("Interrupt", directory, session.id);
      await call("Delete", directory, session.id);
      state.threads.delete(activeKey(directory, session.id));
      state.drafts.delete(activeKey(directory, session.id));
      saved.set("draft:" + activeKey(directory, session.id), "");
      if (state.directory === directory && state.session === session.id) activate(directory);
      await fetchSessions(directory);
    }, tr("Delete session")), { glyph: "trash-2", className: "picker-item destructive" }),
  );
  wrap.append(list);
}

const panels = () => [
  ["files", tr("Files"), "file-text"],
  ["changes", tr("Changes"), "git-compare"],
  ["tools", tr("Skills and connections"), "plug"],
];
function renderRail() {
  rail.setAttribute("aria-label", tr("Side panels"));
  rail.replaceChildren();
  for (const [id, label, glyph] of panels()) {
    const entry = button(label, () => setPanel(state.panel === id ? "" : id), { glyph, onlyIcon: true, className: "rail-button" });
    entry.classList.toggle("selected", state.panel === id);
    entry.setAttribute("aria-pressed", String(state.panel === id));
    rail.append(entry);
  }
}

function setPanel(panel) {
  if (state.panel !== panel) state.editing = false;
  state.panel = panel;
  renderRail();
  void renderPanel();
}

function previewFile(path) {
  state.previewPath = path;
  state.editing = false;
  setPanel("files");
}

let panelGeneration = 0;
async function renderPanel() {
  const { panel, directory, session, previewPath, filePath } = state;
  const generation = ++panelGeneration;
  const stillCurrent = () => generation === panelGeneration;
  right.hidden = !panel;
  if (!panel) return;
  rightBody.classList.toggle("flush", panel === "files" && !!previewPath);
  rightTop.replaceChildren();
  if (panel === "files" && previewPath) {
    rightTop.append(iconButton(tr("Back to files"), "chevron-left", () => { state.previewPath = ""; state.editing = false; void renderPanel(); }));
  }
  rightTop.append(node("h2", "truncate", panel === "files" && previewPath ? previewPath.split("/").at(-1) : panels().find(([id]) => id === panel)[1]));
  rightTop.append(node("span", "spacer"));
  if (panel === "files" && directory) {
    const path = previewPath || filePath;
    rightTop.append(iconButton(tr("Show in Finder"), "folder-open", () => load("Reveal", directory, path)));
  }
  rightTop.append(iconButton(tr("Close panel"), "x", () => setPanel("")));
  rightBody.replaceChildren(node("p", "empty-panel", tr("Loading…")));
  if (!directory) {
    rightBody.replaceChildren(node("p", "empty-panel", tr("Choose a workspace to get started.")));
    return;
  }
  try {
    if (panel === "files") {
      if (previewPath) {
        const preview = await call("Preview", directory, previewPath);
        if (stillCurrent()) renderFilePreview(preview, previewPath);
      } else {
        const entries = await call(tr("Files"), directory, filePath);
        if (stillCurrent()) renderFiles(entries || [], filePath);
      }
    } else if (panel === "changes") {
      const changes = session ? await call("Diffs", directory, session) : [];
      if (stillCurrent()) renderChanges(changes || []);
    } else {
      const catalog = await call("Catalog", directory);
      if (stillCurrent()) renderCatalog(catalog);
    }
  } catch (error) {
    if (stillCurrent()) rightBody.replaceChildren(node("p", "empty-panel destructive", errorLabel(error?.message || String(error))));
  }
}

function renderFiles(entries, path) {
  rightBody.replaceChildren();
  if (path !== ".") {
    const parent = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : ".";
    rightBody.append(button(path, () => { state.filePath = parent; void renderPanel(); }, { glyph: "chevron-left", className: "file-row", title: tr("Parent folder") }));
  }
  if (!entries.length) rightBody.append(node("p", "empty-panel", tr("This folder is empty.")));
  for (const entry of entries) {
    const row = button(entry.name, () => {
      if (entry.directory) {
        state.filePath = entry.path;
        void renderPanel();
      } else previewFile(entry.path);
    }, { glyph: entry.directory ? "folder" : "file-text", className: "file-row" });
    row.lastElementChild.className = "truncate";
    if (entry.directory) row.append(node("span", "spacer"), icon("chevron-right"));
    rightBody.append(row);
  }
}

function renderFilePreview(preview, path) {
  rightBody.replaceChildren();
  if (preview.binary) {
    rightBody.append(node("p", "empty-panel", tr("Open this file in Finder to view it.")));
    return;
  }
  if (preview.partial) rightBody.append(node("p", "empty-panel", tr("Showing a partial preview.")));
  if (preview.revision) {
    const edit = button(tr("Edit"), () => editFile(preview, path), { glyph: "pen-line", className: "pill" });
    edit.dataset.editFile = "";
    rightTop.insertBefore(edit, rightTop.lastElementChild.previousElementSibling);
  }
  if (preview.rows?.length) {
    const table = node("table", "preview-table");
    preview.rows.forEach((values, index) => {
      const row = table.insertRow();
      for (const value of values) row.append(node(index ? "td" : "th", "", value));
    });
    rightBody.append(table);
  } else if (/\.(md|markdown)$/i.test(path)) {
    const rendered = markdown(preview.text, openLink);
    rendered.classList.add("preview-markdown");
    rightBody.append(rendered);
  } else {
    rightBody.append(node("pre", "preview-text", preview.text));
  }
}

function editFile(preview, path) {
  const directory = state.directory;
  state.editing = true;
  const tableRows = preview.rows?.map((row) => [...row]);
  const editor = node("textarea", "file-editor");
  editor.setAttribute("aria-label", tr("File contents"));
  editor.value = preview.text;
  const actions = node("form", "file-edit-actions");
  const editorBody = node("div");
  if (tableRows?.length) {
    const table = node("table", "preview-table editing-table");
    editorBody.append(table);
    const redraw = () => {
      table.replaceChildren();
      const columns = Math.max(...tableRows.map((row) => row.length));
      tableRows.forEach((values, rowIndex) => {
        const row = table.insertRow();
        for (let column = 0; column < columns; column++) {
          const cell = node(rowIndex ? "td" : "th");
          const value = input("text", values[column] ?? "");
          value.setAttribute("aria-label", tr("Row {row}, column {column}", { row: rowIndex + 1, column: column + 1 }));
          value.addEventListener("input", () => {
            while (values.length < column) values.push("");
            values[column] = value.value;
          });
          cell.append(value);
          row.append(cell);
        }
      });
    };
    actions.append(
      button(tr("Row"), () => {
        tableRows.push(Array(Math.max(...tableRows.map((row) => row.length))).fill(""));
        redraw();
        table.rows[table.rows.length - 1].querySelector("input")?.focus();
      }, { glyph: "plus" }),
      button(tr("Column"), () => {
        const width = Math.max(...tableRows.map((row) => row.length));
        for (const row of tableRows) while (row.length <= width) row.push("");
        redraw();
      }, { glyph: "plus" }),
    );
    redraw();
  } else {
    editorBody.append(editor);
  }
  const discard = () => { state.editing = false; void renderPanel(); };
  const save = button(tr("Save"), null, { className: "primary" });
  save.type = "submit";
  actions.append(node("span", "spacer"), button(tr("Discard"), discard), save);
  actions.addEventListener("submit", async (event) => {
    event.preventDefault();
    save.disabled = true;
    try {
      await call("SaveFile", directory, path, preview.revision, editor.value, tableRows?.length ? tableRows : null);
      if (state.directory === directory && state.previewPath === path && state.panel === "files") discard();
    } catch (error) {
      report(error);
      save.disabled = false;
    }
  });
  rightTop.querySelector("[data-edit-file]")?.remove();
  rightBody.replaceChildren(actions, editorBody);
  if (!tableRows?.length) editor.focus();
}

function renderChanges(changes) {
  rightBody.replaceChildren();
  if (!changes.length) rightBody.append(node("p", "empty-panel", state.session ? tr("No file changes in this session.") : tr("Start a task to see its file changes.")));
  for (const change of changes) {
    const details = node("details", "tool");
    const summary = node("summary");
    summary.append(icon("file-text"), node("span", "truncate", change.file), node("span", "spacer"),
      node("small", "diff-add", "+" + change.additions), node("small", "diff-remove", "−" + change.deletions));
    const patch = node("pre", "preview-text");
    for (const line of (change.patch || "").split("\n")) {
      patch.append(node("span", "diff-line " + (line.startsWith("+") ? "diff-add" : line.startsWith("-") ? "diff-remove" : ""), line));
    }
    details.append(summary, patch);
    rightBody.append(details);
  }
}

function renderCatalog(catalog) {
  rightBody.replaceChildren(node("div", "legend", tr("Connections")));
  for (const item of catalog.connections || []) {
    const status = item.status.status || item.status.type || "";
    const row = button(item.title || item.name, () => connectionActions(item, status), { glyph: "plug", className: "file-row" });
    row.append(node("small", "", tr(status)));
    if (status === "pending") row.dataset.pendingMcp = "";
    rightBody.append(row);
  }
  rightBody.append(
    button(tr("Add MCP connection"), showConnectionForm, { glyph: "plus", className: "file-row" }),
    node("div", "legend", tr("Skills")),
  );
  for (const skill of catalog.skills || []) {
    const row = button(skill.name || skill.id, () => showSkill(skill), { glyph: "sparkles", className: "file-row", title: skill.description });
    rightBody.append(row);
  }
  if (!catalog.skills?.length) rightBody.append(node("p", "empty-panel", tr("No skills available for this workspace.")));
}

function showSkill(skill) {
  const wrap = showDialog(skill.name || skill.id);
  wrap.append(node("p", "", skill.description || ""));
  if (skill.content) wrap.append(markdown(skill.content, openLink));
  const actions = node("div", "actions");
  actions.append(button(tr("Use in task"), () => {
    const draft = draftFor();
    if (!draft.skills.some((item) => item.id === skill.id)) draft.skills.push({ id: skill.id, name: skill.name || skill.id });
    dialog.close();
    renderComposerMeta();
    composerInput.focus();
  }, { glyph: "plus", className: "primary" }));
  wrap.append(actions);
}

function showConnectionForm() {
  const name = input("text", "", tr("Connection name"));
  name.required = true;
  const address = input("url", "", "https://example.com/mcp");
  address.required = true;
  const directory = state.directory;
  promptForm(tr("Add MCP connection"), [field(tr("Name"), name), field(tr("Server URL"), address)], async () => {
    await call("AddMCP", directory, name.value, address.value);
    void renderPanel();
  }, tr("Add connection"));
}

function connectionActions(item, status) {
  const content = showDialog(item.title || item.name);
  const directory = state.directory;
  content.append(node("p", "", item.status.error || status || tr("MCP connection")));
  const actions = node("div", "actions");
  const configure = async (action) => {
    try {
      await call("ConfigureMCP", directory, item.name, action);
      dialog.close();
      void renderPanel();
    } catch (error) { report(error); }
  };
  if (item.managed) actions.append(button(tr("Remove"), () => configure("remove"), { className: "secondary destructive" }));
  actions.append(button(status === "connected" ? tr("Disconnect") : tr("Connect"), () => configure(status === "connected" ? "disconnect" : "connect"), { className: "primary" }));
  content.append(actions);
}

function languageSettings() {
  const section = node("section", "settings-section");
  section.append(node("h3", "", tr("Language")));
  const select = node("select");
  select.id = "interface-language";
  select.setAttribute("aria-label", tr("Interface language"));
  select.add(new Option("简体中文", "zh-CN"));
  select.add(new Option("English", "en"));
  select.value = state.language;
  select.addEventListener("change", async () => {
    select.disabled = true;
    const language = select.value;
    try {
      await call("SetLanguage", language);
      state.language = setLanguage(language);
      renderShell();
      if (!state.editing) void renderPanel();
      void showSettings();
      dialog.querySelector("#interface-language")?.focus();
    } catch (error) {
      select.value = state.language;
      report(error);
    } finally {
      select.disabled = false;
    }
  });
  section.append(field(tr("Interface language"), select));
  return section;
}

async function showSettings() {
  const wrap = showDialog(tr("Settings"));
  const engine = node("section", "settings-section");
  const details = node("p", "", state.engineError ? errorLabel(state.engineError) : (state.version ? tr("OpenCode {version} · Local engine", { version: state.version }) : tr("Connecting to OpenCode…")));
  engine.append(node("h3", "", tr("Runtime")), details,
    button(tr("Check connection"), async () => {
      await checkVersion();
      details.textContent = state.engineError ? errorLabel(state.engineError) : tr("OpenCode {version} · Local engine", { version: state.version });
    }, { glyph: "rotate-ccw", className: "secondary" }));
  const providers = node("section", "settings-section");
  providers.append(node("h3", "", tr("Model providers")));
  if (state.directory) {
    providers.append(node("p", "", tr("Loading providers…")),
      button(tr("Connect a provider"), showProviders, { glyph: "plus", className: "secondary" }));
    const directory = state.directory;
    void call("Catalog", directory).then((catalog) => {
      const list = catalog.providers?.map((item) => item.name).join(", ");
      providers.querySelector("p").textContent = list ? tr("Available · {providers}", { providers: list }) : tr("No connected providers.");
    }).catch((error) => { providers.querySelector("p").textContent = errorLabel(error.message); });
  } else providers.append(node("p", "", tr("Add a workspace to see its providers.")));
  const keys = node("section", "settings-section");
  keys.append(node("h3", "", tr("Keyboard shortcuts")), node("p", "", tr("⌘N New task · ⇧⌘F Search · ⌘, Settings · ⇧Enter New line")));
  wrap.append(languageSettings(), engine, providers, keys);
}

async function showProviders() {
  const directory = state.directory;
  if (!directory) return;
  const wrap = showDialog(tr("Connect a provider"));
  wrap.append(node("p", "", tr("Loading providers…")));
  try {
    const providers = await call("Integrations", directory);
    if (!wrap.isConnected || !dialog.open) return;
    picker(tr("Connect a provider"), (providers || []).map((item) => ({ key: item.id, label: item.name, detail: item.id })), (item) => {
      const secret = input("password", "", tr("API key"));
      secret.required = true;
      secret.autocomplete = "off";
      promptForm(tr("Connect {provider}", { provider: item.label }), [field(tr("API key"), secret)], async () => {
        await call("SaveKey", item.key, secret.value);
        secret.value = "";
        await loadOptions(directory, true);
        report(tr("{provider} connected", { provider: item.label }));
      }, tr("Connect"));
    }, "", tr("Search providers…"));
  } catch (error) {
    if (wrap.isConnected) wrap.replaceChildren(node("p", "destructive", errorLabel(error.message)));
  }
}

async function checkVersion() {
  try {
    state.version = await call("Version");
    state.engineError = "";
    state.connected = true;
  } catch (error) {
    state.engineError = error?.message || String(error);
  }
  renderFooter();
}

document.addEventListener("keydown", (event) => {
  if (!event.metaKey) return;
  if (event.key.toLowerCase() === "n" && !event.shiftKey) {
    event.preventDefault();
    if (dialog.open) dialog.close();
    void newTask();
  }
  if (event.key.toLowerCase() === "f" && event.shiftKey) {
    event.preventDefault();
    showSearch();
  }
  if (event.key === ",") {
    event.preventDefault();
    void showSettings();
  }
});

async function boot() {
  renderShell();
  if (!window.mygo) {
    state.engineError = tr("This interface runs in MyGo Work");
    renderFooter();
    return;
  }
  window.mygo.on("agentUpdate", receipt);
  state.language = setLanguage(await load("Language"));
  renderShell();
  void checkVersion();
  state.folders = await load("Workspaces") || [];
  const selected = state.folders.find((entry) => entry.directory === saved.get("lastWorkspace")) || state.folders[0];
  if (selected) activate(selected.directory, saved.get("lastSession:" + selected.directory));
  else renderSidebar();
  setInterval(() => {
    const { directory, session } = state;
    if (!directory || state.engineError) return;
    void refreshActive(directory);
    if (session) void fetchThread(directory, session);
    if (state.panel === "tools" && rightBody.querySelector("[data-pending-mcp]") && !dialog.open) void renderPanel();
  }, 2600);
}

void boot();
