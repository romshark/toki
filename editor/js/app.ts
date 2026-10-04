export {}; // Ensure this file is treated as a module.

// --- Sidebar state (tab-scoped) ---

const SIDEBAR_ID = "editor-sidebar";

type NeoSidebar = HTMLElement & { toggle(): void };

function sidebarEl(): NeoSidebar | null {
  return document.getElementById(SIDEBAR_ID) as NeoSidebar | null;
}

// Restore the persisted collapsed state as soon as the host appears.
// `open="false"` is <neo-sidebar>'s command form for "closed"; omitting
// the attribute would instead leave the auto-open behaviour in charge.
if (sessionStorage.getItem("toki-sidebar") === "false") {
  new MutationObserver((_, obs) => {
    const sidebar = sidebarEl();
    if (sidebar) {
      sidebar.setAttribute("open", "false");
      obs.disconnect();
    }
  }).observe(document.documentElement, { childList: true, subtree: true });
}

(window as any).toggleSidebar = function toggleSidebar() {
  sidebarEl()?.toggle();
};

// Persist from the component's own events rather than from the toggle
// call, so state stays correct when the sidebar closes some other way
// (Escape, backdrop click, swipe-to-dismiss).
for (const [event, state] of [
  ["neo-sidebar-open", "true"],
  ["neo-sidebar-close", "false"],
] as const) {
  document.addEventListener(event, (evt) => {
    if ((evt.target as Element | null)?.id === SIDEBAR_ID) {
      sessionStorage.setItem("toki-sidebar", state);
    }
  });
}

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
  }
});
