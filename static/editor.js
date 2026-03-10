(function () {
    "use strict";

    var editBtn = document.getElementById("edit-btn");
    var saveBtn = document.getElementById("save-btn");
    var cancelBtn = document.getElementById("cancel-btn");
    var editorToolbar = document.getElementById("editor-toolbar");
    var article = document.querySelector("article");
    var editorEl = document.getElementById("editor-container");
    var mdPath = editBtn ? editBtn.getAttribute("data-md-path") : null;
    var textarea = null;

    if (!editBtn || !article || !editorEl || !mdPath) return;

    function enterEditMode() {
        editBtn.style.display = "none";
        editorToolbar.style.display = "flex";
        article.style.display = "none";
        editorEl.style.display = "block";

        // Fetch raw markdown
        var xhr = new XMLHttpRequest();
        xhr.open("GET", "/api/page?path=" + encodeURIComponent(mdPath));
        xhr.onload = function () {
            if (xhr.status !== 200) {
                alert("Failed to load markdown source (HTTP " + xhr.status + ")");
                exitEditMode();
                return;
            }
            initEditor(xhr.responseText);
        };
        xhr.onerror = function () {
            alert("Network error loading markdown source");
            exitEditMode();
        };
        xhr.send();
    }

    function initEditor(markdown) {
        textarea = document.createElement("textarea");
        textarea.className = "editor-textarea";
        textarea.value = markdown;
        textarea.spellcheck = false;
        editorEl.innerHTML = "";
        editorEl.appendChild(textarea);
        textarea.focus();
    }

    function exitEditMode() {
        editBtn.style.display = "";
        editorToolbar.style.display = "none";
        article.style.display = "";
        editorEl.style.display = "none";
        editorEl.innerHTML = "";
        textarea = null;
    }

    function saveDocument() {
        if (!textarea) return;

        var content = textarea.value;
        saveBtn.disabled = true;
        saveBtn.textContent = "Saving...";

        var xhr = new XMLHttpRequest();
        xhr.open("PUT", "/api/page?path=" + encodeURIComponent(mdPath));
        xhr.setRequestHeader("Content-Type", "text/markdown; charset=utf-8");
        xhr.onload = function () {
            saveBtn.disabled = false;
            saveBtn.textContent = "Save";
            if (xhr.status === 200) {
                // Wait briefly for the file watcher to rebuild, then reload
                setTimeout(function () {
                    window.location.reload();
                }, 800);
            } else {
                alert("Save failed (HTTP " + xhr.status + ")");
            }
        };
        xhr.onerror = function () {
            saveBtn.disabled = false;
            saveBtn.textContent = "Save";
            alert("Network error saving document");
        };
        xhr.send(content);
    }

    // ---- Completed tickbox ----
    var completedCb = document.getElementById("completed-cb");
    if (completedCb) {
        completedCb.addEventListener("change", function () {
            var cb = completedCb;
            var label = cb.closest(".completed-toggle");
            if (label) {
                label.classList.toggle("completed-toggle-on", cb.checked);
                var txt = label.querySelector(".completed-toggle-label");
                if (txt) txt.textContent = cb.checked ? "Completed" : "Mark complete";
            }
            function revert() {
                cb.checked = !cb.checked;
                cb.disabled = false;
                if (label) {
                    label.classList.toggle("completed-toggle-on", cb.checked);
                    var t = label.querySelector(".completed-toggle-label");
                    if (t) t.textContent = cb.checked ? "Completed" : "Mark complete";
                }
            }
            cb.disabled = true;
            var xhr = new XMLHttpRequest();
            xhr.open("GET", "/api/page?path=" + encodeURIComponent(mdPath));
            xhr.onload = function () {
                if (xhr.status !== 200) { revert(); return; }
                var raw = xhr.responseText;
                var updated = toggleCompleted(raw, cb.checked);
                var xhr2 = new XMLHttpRequest();
                xhr2.open("PUT", "/api/page?path=" + encodeURIComponent(mdPath));
                xhr2.setRequestHeader("Content-Type", "text/markdown; charset=utf-8");
                xhr2.onload = function () {
                    cb.disabled = false;
                    if (xhr2.status === 200) {
                        setTimeout(function () { window.location.reload(); }, 800);
                    } else { revert(); }
                };
                xhr2.onerror = function () { revert(); };
                xhr2.send(updated);
            };
            xhr.onerror = function () { revert(); };
            xhr.send();
        });
    }

    function toggleCompleted(raw, checked) {
        var lines = raw.split("\n");
        if (lines[0] !== "---") return raw;
        var end = -1;
        for (var i = 1; i < lines.length; i++) {
            if (lines[i] === "---") { end = i; break; }
        }
        if (end < 0) return raw;

        var today = new Date().toISOString().slice(0, 10);
        var found = false;
        for (var j = 1; j < end; j++) {
            if (/^completed\s*:/.test(lines[j])) {
                found = true;
                lines[j] = checked ? "completed: " + today : "completed:";
                break;
            }
        }
        if (!found && checked) {
            // Insert before closing ---
            lines.splice(end, 0, "completed: " + today);
        }
        return lines.join("\n");
    }

    editBtn.addEventListener("click", enterEditMode);
    cancelBtn.addEventListener("click", exitEditMode);
    saveBtn.addEventListener("click", saveDocument);

    // Ctrl/Cmd+S to save while editing
    document.addEventListener("keydown", function (e) {
        if ((e.ctrlKey || e.metaKey) && e.key === "s" && textarea) {
            e.preventDefault();
            saveDocument();
        }
    });

    // Tab key inserts spaces instead of moving focus
    document.addEventListener("keydown", function (e) {
        if (e.key === "Tab" && textarea && document.activeElement === textarea) {
            e.preventDefault();
            var start = textarea.selectionStart;
            var end = textarea.selectionEnd;
            textarea.value = textarea.value.substring(0, start) + "    " + textarea.value.substring(end);
            textarea.selectionStart = textarea.selectionEnd = start + 4;
        }
    });
})();
