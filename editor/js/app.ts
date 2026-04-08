export {}; // Ensure this file is treated as a module.

// --- Sidebar state (tab-scoped) ---

if (sessionStorage.getItem("toki-sidebar") === "false") {
  new MutationObserver((_, obs) => {
    const sidebar = document.getElementById("editor-sidebar");
    if (sidebar) {
      sidebar.setAttribute("data-initial-open", "false");
      sidebar.setAttribute("aria-hidden", "true");
      sidebar.setAttribute("inert", "");
      obs.disconnect();
    }
  }).observe(document.documentElement, { childList: true, subtree: true });
}

(window as any).toggleSidebar = function toggleSidebar() {
  const isOpen = sessionStorage.getItem("toki-sidebar") !== "false";
  sessionStorage.setItem("toki-sidebar", isOpen ? "false" : "true");
  document.dispatchEvent(new CustomEvent(
    "basecoat:sidebar", { detail: { id: "editor-sidebar" } },
  ));
};

// --- OS theme preference change ---
// When the user has "system" theme and toggles OS dark mode,
// update the current page without reloading.
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
  const theme = document.cookie.match(/(?:^|;\s*)toki-theme=([^;]*)/)?.[1] || "system";
  if (theme === "system") {
    document.documentElement.classList.toggle(
      "dark",
      matchMedia("(prefers-color-scheme: dark)").matches,
    );
    document.dispatchEvent(new CustomEvent("toki-theme-change"));
  }
});

// --- <toki-editor> integration ---

(window as any).syncEditorValues = function syncEditorValues(values: Record<string, string>) {
  for (const id in values) {
    const el = document.getElementById(id) as any;
    if (el && el.value !== undefined) {
      el.value = values[id];
    }
  }
};

(window as any).resetEditorValue = function resetEditorValue(editorId: string, value: string) {
  const el = document.getElementById(editorId) as any;
  if (el && el.value !== undefined) el.value = value;
};

(window as any).getEditorValue = function getEditorValue(id: string): string {
  const el = document.getElementById(id) as any;
  return el ? el.value : "";
};

(window as any).getOrCreateInstanceID = function getOrCreateInstanceID(storageKey: string): string {
  let id = sessionStorage.getItem(storageKey);
  if (id) return id;
  if (window.crypto && typeof window.crypto.randomUUID === "function") {
    id = window.crypto.randomUUID();
  } else {
    id = Date.now().toString(36) + Math.random().toString(36).slice(2);
  }
  sessionStorage.setItem(storageKey, id);
  return id;
};

// --- Filter signal sync ---

(window as any).syncShownLocales = function syncShownLocales(): string {
  const switches = document.querySelectorAll<HTMLInputElement>('[data-bind^="showlocales."]');
  const shown: string[] = [];
  switches.forEach((sw) => {
    if (sw.checked) {
      const locale = sw.getAttribute("data-bind")!.replace("showlocales.", "");
      shown.push(locale);
    }
  });
  return shown.length > 0 ? shown.join(",") : "-";
};

(window as any).syncShownDomains = function syncShownDomains(): string {
  const switches = document.querySelectorAll<HTMLInputElement>('[data-bind^="showdomains."]');
  const shown: string[] = [];
  switches.forEach((sw) => {
    if (sw.checked) {
      const key = sw.getAttribute("data-bind")!.replace("showdomains.", "");
      shown.push(key.replace(/_/g, "."));
    }
  });
  return shown.length > 0 ? shown.join(",") : "-";
};
