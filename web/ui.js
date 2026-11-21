import { marked } from "./vendor/marked.js";

export const node = (tag, className, text) => {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
};

export const icon = (name) => {
  const element = node("span", "icon i-" + name);
  element.setAttribute("aria-hidden", "true");
  return element;
};

export function button(label, action, { glyph, className = "pill", onlyIcon = false, title = label } = {}) {
  const element = node("button", className);
  element.type = "button";
  if (glyph) element.append(icon(glyph));
  if (onlyIcon) element.setAttribute("aria-label", label);
  else element.append(node("span", "", label));
  if (title) element.title = title;
  element.onclick = action;
  return element;
}

export const iconButton = (label, glyph, action) => button(label, action, { glyph, className: "icon-button", onlyIcon: true });

export function field(title, input) {
  const wrapper = node("label", "field");
  wrapper.append(node("span", "", title), input);
  return wrapper;
}

export function input(type = "text", value = "", placeholder = "") {
  const element = node("input");
  element.type = type;
  element.value = value;
  element.placeholder = placeholder;
  return element;
}

const tags = new Set("A B BLOCKQUOTE BR CODE DEL EM H1 H2 H3 H4 H5 H6 HR I LI OL P PRE S STRONG TABLE TBODY TD TH THEAD TR UL".split(" "));
export function markdown(text, openLink) {
  const template = node("template");
  template.innerHTML = marked.parse(text || "", { gfm: true });
  const result = node("div", "markdown");
  function append(parent, source) {
    if (source.nodeType === Node.TEXT_NODE) {
      parent.append(document.createTextNode(source.textContent));
      return;
    }
    if (source.nodeType !== Node.ELEMENT_NODE) return;
    if (!tags.has(source.tagName)) {
      if (source.tagName === "IMG") {
        parent.append(document.createTextNode(source.getAttribute("alt") || ""));
      } else if (!["SCRIPT", "STYLE", "TEMPLATE", "IFRAME", "SVG"].includes(source.tagName)) {
        for (const child of source.childNodes) append(parent, child);
      }
      return;
    }
    const element = node(source.localName);
    if (source.tagName === "A") {
      const href = source.getAttribute("href");
      if (/^https?:\/\//i.test(href || "")) {
        element.href = href;
        element.addEventListener("click", (event) => {
          event.preventDefault();
          openLink(href);
        });
      }
    }
    if (source.tagName === "OL" && /^\d+$/.test(source.getAttribute("start") || "")) {
      element.start = Number(source.getAttribute("start"));
    }
    for (const child of source.childNodes) append(element, child);
    parent.append(element);
  }
  for (const child of template.content.childNodes) append(result, child);
  return result;
}

export const saved = {
  get(key, fallback = "") {
    try { return localStorage.getItem(key) ?? fallback; } catch { return fallback; }
  },
  set(key, value) {
    try { localStorage.setItem(key, value); } catch { /* optional UI preference */ }
  },
};
