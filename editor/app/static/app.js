const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");

// --- Sidebar state (tab-scoped) ---

// Sidebar: persist open/closed state in sessionStorage (tab-scoped).
// MutationObserver sets data-initial-open before basecoat's deferred script.
if (sessionStorage.getItem("toki-sidebar") === "false") {
	new MutationObserver(function(_, obs) {
		var sidebar = document.getElementById("editor-sidebar");
		if (sidebar) {
			sidebar.setAttribute("data-initial-open", "false");
			sidebar.setAttribute("aria-hidden", "true");
			sidebar.setAttribute("inert", "");
			obs.disconnect();
		}
	}).observe(document.documentElement, { childList: true, subtree: true });
}

function toggleSidebar() {
	var isOpen = sessionStorage.getItem("toki-sidebar") !== "false";
	sessionStorage.setItem("toki-sidebar", isOpen ? "false" : "true");
	document.dispatchEvent(new CustomEvent(
		"basecoat:sidebar", { detail: { id: "editor-sidebar" } },
	));
}

// --- Theme management ---

function getStoredTheme() {
	return localStorage.getItem("toki-theme") || "system";
}

function applyTheme(theme) {
	var root = document.documentElement;
	if (theme === "dark") {
		root.classList.add("dark");
	} else if (theme === "light") {
		root.classList.remove("dark");
	} else {
		if (prefersDark.matches) {
			root.classList.add("dark");
		} else {
			root.classList.remove("dark");
		}
	}
}

function setTheme(theme) {
	localStorage.setItem("toki-theme", theme);
	applyTheme(theme);
	updateThemeCards();
}

function updateThemeCards() {
	var current = getStoredTheme();
	document.querySelectorAll(".theme-card").forEach(function(card) {
		card.setAttribute("aria-pressed", card.dataset.theme === current);
	});
}

function initThemeCards() {
	updateThemeCards();
	updateFontCards();
	updateFontSizeCards();
}

// --- Font management ---

var fontDefaults = {
	"toki-ui-font": "system",
	"toki-editor-font": "mono-system",
};

var fontFamilies = {
	// UI fonts
	"system": "",
	"georgia": "Georgia, 'Times New Roman', serif",
	"helvetica": "'Helvetica Neue', Helvetica, Arial, sans-serif",
	// Editor fonts
	"mono-system": "ui-monospace, 'SF Mono', 'Cascadia Code', 'Segoe UI Mono', monospace",
	"mono-firacode": "'Fira Code', monospace",
	"mono-monaco": "Monaco, 'Consolas', monospace",
	"mono-courier": "'Courier New', Courier, monospace",
};

function getStoredFont(key) {
	var v = localStorage.getItem(key) || fontDefaults[key];
	if (!(v in fontFamilies)) v = fontDefaults[key];
	return v;
}

function setFont(key, value) {
	localStorage.setItem(key, value);
	applyFont(key, value);
	updateFontCards();
}

function applyFont(key, value) {
	var family = fontFamilies[value] || "";
	if (key === "toki-ui-font") {
		document.documentElement.style.setProperty("--font-ui", family || "");
	} else if (key === "toki-editor-font") {
		document.documentElement.style.setProperty("--font-editor", family || "");
	}
}

function updateFontCards() {
	var uiFont = getStoredFont("toki-ui-font");
	var editorFont = getStoredFont("toki-editor-font");
	document.querySelectorAll(".font-card").forEach(function(card) {
		var key = card.dataset.fontKey;
		var val = card.dataset.fontValue;
		var current = key === "toki-ui-font" ? uiFont : editorFont;
		card.setAttribute("aria-pressed", val === current);
	});
}

// --- Font size management ---

var fontSizeDefaults = {
	"toki-ui-font-size": "default",
	"toki-editor-font-size": "default",
};

var fontSizes = {
	"very-small": "0.8rem",
	"small": "0.9rem",
	"default": "",
	"big": "1.1rem",
	"bigger": "1.25rem",
};

function getStoredFontSize(key) {
	return localStorage.getItem(key) || fontSizeDefaults[key];
}

function setFontSize(key, value) {
	localStorage.setItem(key, value);
	applyFontSize(key, value);
	updateFontSizeCards();
}

function applyFontSize(key, value) {
	var size = fontSizes[value] || "";
	if (key === "toki-ui-font-size") {
		document.documentElement.style.setProperty("--font-size-ui", size || "");
	} else if (key === "toki-editor-font-size") {
		document.documentElement.style.setProperty("--font-size-editor", size || "");
	}
}

function updateFontSizeCards() {
	var uiSize = getStoredFontSize("toki-ui-font-size");
	var editorSize = getStoredFontSize("toki-editor-font-size");
	document.querySelectorAll(".font-size-card").forEach(function(card) {
		var key = card.dataset.sizeKey;
		var val = card.dataset.sizeValue;
		var current = key === "toki-ui-font-size" ? uiSize : editorSize;
		card.setAttribute("aria-pressed", val === current);
	});
}

applyFont("toki-ui-font", getStoredFont("toki-ui-font"));
applyFont("toki-editor-font", getStoredFont("toki-editor-font"));
applyFontSize("toki-ui-font-size", getStoredFontSize("toki-ui-font-size"));
applyFontSize("toki-editor-font-size", getStoredFontSize("toki-editor-font-size"));

applyTheme(getStoredTheme());

new MutationObserver(function() {
	var theme = getStoredTheme();
	var isDark = document.documentElement.classList.contains("dark");
	var shouldBeDark = theme === "dark" || (theme === "system" && prefersDark.matches);
	if (shouldBeDark !== isDark) {
		applyTheme(theme);
	}
}).observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });

prefersDark.addEventListener("change", () => {
	applyTheme(getStoredTheme());
});

// --- History navigation buttons ---

if (window.navigation) {
	function updateNavButtons() {
		document.querySelectorAll(".nav-back").forEach(function(b) {
			b.disabled = !navigation.canGoBack;
		});
		document.querySelectorAll(".nav-forward").forEach(function(b) {
			b.disabled = !navigation.canGoForward;
		});
	}
	navigation.addEventListener("navigatesuccess", updateNavButtons);
	navigation.addEventListener("currententrychange", updateNavButtons);
	updateNavButtons();
}

// --- <toki-editor> integration ---

// Autosave: listen for toki-change events from <toki-editor> components.
document.addEventListener("toki-change", function(e) {
	var d = e.detail;
	if (d.tikid && d.locale) {
		window._autosave = { tikid: d.tikid, locale: d.locale, value: d.value };
		var btn = document.getElementById("autosave-trigger");
		if (btn) btn.click();
	}
});

// syncEditorValues sets server-side values on <toki-editor> elements.
// Called after morphdom patches to keep editors with data-ignore-morph in sync.
function syncEditorValues(values) {
	for (var id in values) {
		var el = document.getElementById(id);
		if (el && el.value !== undefined) {
			el.value = values[id];
		}
	}
}

// resetEditorValue forces a single editor to a specific value (used after reset).
function resetEditorValue(editorId, value) {
	var el = document.getElementById(editorId);
	if (el && el.value !== undefined) el.value = value;
}

function getEditorValue(id) {
	var el = document.getElementById(id);
	return el ? el.value : '';
}

function getOrCreateInstanceID(storageKey) {
	var id = sessionStorage.getItem(storageKey);
	if (id) return id;
	if (window.crypto && typeof window.crypto.randomUUID === "function") {
		id = window.crypto.randomUUID();
	} else {
		id = Date.now().toString(36) + Math.random().toString(36).slice(2);
	}
	sessionStorage.setItem(storageKey, id);
	return id;
}

// Sync the showlocales map signal into a comma-separated string for the URL query param.
function syncShownLocales() {
	var switches = document.querySelectorAll('[data-bind^="showlocales."]');
	var shown = [];
	switches.forEach(function(sw) {
		if (sw.checked) {
			var locale = sw.getAttribute('data-bind').replace('showlocales.', '');
			shown.push(locale);
		}
	});
	return shown.length > 0 ? shown.join(',') : '-';
}

// Sync the showdomains map signal into a comma-separated string for the URL query param.
// Signal keys use underscores (showdomains.myapp_api) but the value uses dots (myapp.api).
function syncShownDomains() {
	var switches = document.querySelectorAll('[data-bind^="showdomains."]');
	var shown = [];
	switches.forEach(function(sw) {
		if (sw.checked) {
			var key = sw.getAttribute('data-bind').replace('showdomains.', '');
			shown.push(key.replace(/_/g, '.'));
		}
	});
	return shown.length > 0 ? shown.join(',') : '-';
}
