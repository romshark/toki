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
	updateCodeMirrorThemes();
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
	"mono-system": "",
	"mono-menlo": "Menlo, Monaco, 'Cascadia Code', monospace",
	"mono-courier": "'Courier New', Courier, monospace",
};

function getStoredFont(key) {
	return localStorage.getItem(key) || fontDefaults[key];
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

// --- CodeMirror ---

function getCodeMirrorTheme() {
	return document.documentElement.classList.contains("dark")
		? "base16-dark"
		: "base16-light";
}

function updateCodeMirrorThemes() {
	var theme = getCodeMirrorTheme();
	document.querySelectorAll(".CodeMirror").forEach(cmEl => {
		var cm = cmEl.CodeMirror;
		if (cm) cm.setOption("theme", theme);
	});
}

function getServerTextareaValue(el) {
	var serverValue = el.defaultValue;
	if ((serverValue == null || serverValue === "") && el.textContent != null) {
		serverValue = el.textContent;
	}
	if (serverValue == null) {
		serverValue = "";
	}
	return serverValue;
}

function initEditors() {
	var theme = getCodeMirrorTheme();

	document.querySelectorAll("textarea.editor").forEach(el => {
		var serverValue = getServerTextareaValue(el);
		if (el.value !== serverValue) {
			el.value = serverValue;
		}

		if (el.dataset.initialized) {
			var existingCMEl = el.nextElementSibling;
			var existingCM = existingCMEl && existingCMEl.CodeMirror;
			if (existingCM) {
				existingCM.setOption("theme", theme);
				// Skip value sync for focused editors to avoid resetting
				// content while the user is actively typing.
				// Always sync if the window isn't active (user is in another tab).
				if (!existingCM.hasFocus() || !document.hasFocus()) {
					var sv = window._serverValues && window._serverValues[el.id];
					if (sv !== undefined && existingCM.getValue() !== sv) {
						existingCM.state.tokiSyncing = true;
						existingCM.setValue(sv);
						existingCM.state.tokiSyncing = false;
					}
				}
			}
			return;
		}

		var mode = el.dataset.mode;
		var readOnly = el.hasAttribute("readonly");

		var cm = CodeMirror.fromTextArea(el, {
			mode,
			readOnly,
			theme,
			lineNumbers: true,
			inputStyle: "textarea",
			scrollbarStyle: "null",
			specialChars: /[ \u00a0]/g,
			specialCharPlaceholder: function(ch) {
				var span = document.createElement("span");
				if (ch === "\u00a0") {
					span.className = "cm-whitespace cm-nbsp";
					span.textContent = "\u00a0";
				} else {
					span.className = "cm-whitespace cm-space";
					span.textContent = ch;
				}
				return span;
			},
		});

		// Auto-save: debounced POST on change for editable editors.
		if (!readOnly && el.dataset.autosave) {
			var tikid = el.dataset.tikid;
			var locale = el.dataset.locale;
			var saveTimeout;
			cm.on("change", function(instance, change) {
				if (instance.state.tokiSyncing || (change && change.origin === "setValue")) {
					return;
				}
				clearTimeout(saveTimeout);
				saveTimeout = setTimeout(function() {
					instance.save();
					triggerAutoSave(tikid, locale, instance.getValue());
				}, 200);
			});
		}

		el.dataset.initialized = "true";
	});
}

// triggerAutoSave stores values in a global and clicks the hidden autosave button.
// The button's WithBefore expression reads from window._autosave.
function triggerAutoSave(tikid, locale, value) {
	window._autosave = { tikid: tikid, locale: locale, value: value };
	var btn = document.getElementById("autosave-trigger");
	if (btn) btn.click();
}

function getEditorValue(id) {
	var ta = document.getElementById(id);
	if (!ta) return '';
	var cmEl = ta.nextElementSibling;
	if (cmEl && cmEl.CodeMirror) {
		cmEl.CodeMirror.save();
	}
	return ta.value;
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

// Force-update a CodeMirror editor's content (used after reset).
function resetEditorValue(editorId, value) {
	var ta = document.getElementById(editorId);
	if (!ta) return;
	var cmEl = ta.nextElementSibling;
	if (cmEl && cmEl.CodeMirror) {
		cmEl.CodeMirror.state.tokiSyncing = true;
		cmEl.CodeMirror.setValue(value);
		cmEl.CodeMirror.state.tokiSyncing = false;
	}
}
