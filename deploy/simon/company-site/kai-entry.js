(function () {
  "use strict";

  var CONTAINER_ID = "project-links";
  var LINK_ID = "kai-project-link";
  var STYLE_ID = "kai-project-link-style";

  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;

    var style = document.createElement("style");
    style.id = STYLE_ID;
    style.textContent = [
      "#" + CONTAINER_ID + " .kai-project-entry {",
      "  display: inline-flex;",
      "  align-items: center;",
      "  justify-content: center;",
      "  min-height: 48px;",
      "  padding: 0 20px;",
      "  border: 1px solid rgba(255, 255, 255, 0.18);",
      "  border-radius: 14px;",
      "  background: linear-gradient(135deg, #4f8fe8, #7057d9);",
      "  color: #f8fafc;",
      "  font: 700 15px/1 system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;",
      "  letter-spacing: 0.01em;",
      "  text-decoration: none;",
      "  white-space: nowrap;",
      "  box-shadow: 0 8px 24px rgba(39, 67, 131, 0.3);",
      "  transition: transform 180ms ease, filter 180ms ease, box-shadow 180ms ease;",
      "}",
      "#" + CONTAINER_ID + " .kai-project-entry:hover {",
      "  filter: brightness(1.08);",
      "  transform: translateY(-2px);",
      "  box-shadow: 0 10px 28px rgba(39, 67, 131, 0.4);",
      "}",
      "#" + CONTAINER_ID + " .kai-project-entry:active {",
      "  transform: translateY(0) scale(0.98);",
      "}",
      "#" + CONTAINER_ID + " .kai-project-entry:focus-visible {",
      "  outline: 2px solid #8bdcff;",
      "  outline-offset: 3px;",
      "}",
      "@media (max-width: 640px) {",
      "  #" + CONTAINER_ID + " .kai-project-entry {",
      "    min-height: 42px;",
      "    padding: 0 14px;",
      "    font-size: 13px;",
      "    border-radius: 12px;",
      "  }",
      "}",
      "@media (prefers-reduced-motion: reduce) {",
      "  #" + CONTAINER_ID + " .kai-project-entry {",
      "    transition: none;",
      "  }",
      "}"
    ].join("\n");
    document.head.appendChild(style);
  }

  function copySiblingTreatment(link, container) {
    var siblings = container.querySelectorAll("a");
    var reference = siblings.length ? siblings[siblings.length - 1] : null;
    if (!reference) return;

    var sharedClasses = String(reference.className || "").trim();
    link.className = (sharedClasses ? sharedClasses + " " : "") + "kai-project-entry";

    var computed = window.getComputedStyle(reference);
    [
      "background",
      "border",
      "borderRadius",
      "boxShadow",
      "color",
      "fontFamily",
      "fontSize",
      "fontWeight",
      "lineHeight",
      "padding"
    ].forEach(function (property) {
      link.style[property] = computed[property];
    });
  }

  function ensureEntry() {
    var container = document.getElementById(CONTAINER_ID);
    if (!container || document.getElementById(LINK_ID)) return;

    ensureStyle();

    var link = document.createElement("a");
    link.id = LINK_ID;
    link.className = "kai-project-entry";
    link.href = "/kai/";
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.textContent = "KAI影";
    link.setAttribute("aria-label", "在新窗口打开 KAI影");

    copySiblingTreatment(link, container);
    container.appendChild(link);
  }

  function start() {
    ensureEntry();

    var observer = new MutationObserver(ensureEntry);
    observer.observe(document.body, { childList: true, subtree: true });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start, { once: true });
  } else {
    start();
  }
})();
