const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");

function syncDarkClass() {
	document.documentElement.classList.toggle("dark", prefersDark.matches);
}

window.initCodeMirror = function(el) {
	if (typeof el === "function") {
		el = el();
	}
	// el can be the <textarea> or a wrapper; normalize
	const ta = el.tagName === "TEXTAREA" ? el : el.querySelector("textarea.editor");
	if (!ta || ta.dataset.initialized) {
		return;
	}

	const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");
	const theme = prefersDark.matches ? "base16-dark" : "base16-light";

	const cm = CodeMirror.fromTextArea(ta, {
		mode: "icu",
		readOnly: ta.dataset.readonly === "true",
		theme,
		inputStyle: "textarea",
		scrollbarStyle: "null",
	});

	cm.on("blur", cm.save);
	cm.on("change", () => {
		clearTimeout(cm.saveTimeout);
		cm.saveTimeout = setTimeout(cm.save, 100);
	});

	ta.dataset.initialized = "true";
	ta.__cm = cm; // optional handle
};

window.addEventListener("DOMContentLoaded", () => {
	// Apply initial dark class based on system setting.
	syncDarkClass();

	prefersDark.addEventListener("change", () => {
		// React to system theme changes.
		syncDarkClass();
	});
});
