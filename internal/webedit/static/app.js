const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");

function getCurrentTheme() {
	return prefersDark.matches ? "base16-dark" : "base16-light";
}

function updateCodeMirrorTheme() {
	const theme = getCurrentTheme();
	document.querySelectorAll(".CodeMirror").forEach(cmEl => {
		const cm = cmEl.CodeMirror;
		if (cm) cm.setOption("theme", theme);
	});
}

function initEditors() {
	const theme = getCurrentTheme();

	document.querySelectorAll("textarea.editor").forEach(el => {
		// Skip if already initialized.
		if (el.dataset.initialized) return;

		const mode = el.dataset.mode;
		const readOnly = el.dataset.readonly;

		const cm = CodeMirror.fromTextArea(el, {
			mode,
			readOnly,
			theme,
			inputStyle: "textarea",
			scrollbarStyle: "null",
		});

		// Auto-save on blur and change events to keep textarea in sync
		cm.on('blur', cm.save);
		cm.on('change', function () {
			// Debounced save to avoid too frequent updates
			clearTimeout(cm.saveTimeout);
			cm.saveTimeout = setTimeout(cm.save, 100);
		});

		// Mark initialized to avoid re-initialization on the next swap.
		el.dataset.initialized = "true";
	});
}

window.addEventListener("DOMContentLoaded", () => {
	initEditors();
	updateCodeMirrorTheme();

	prefersDark.addEventListener("change", () => {
		// Update code mirror theme on system preference change.
		updateCodeMirrorTheme();
	});

	document.body.addEventListener("htmx:afterSwap", () => {
		// Re-initialize editors after HTMX swaps.
		initEditors();
		updateCodeMirrorTheme();
	});
});
