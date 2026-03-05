(function () {
    "use strict";

    var VDITOR_CDN = "https://unpkg.com/vditor@3.10.8";
    var editBtn = document.getElementById("edit-btn");
    var saveBtn = document.getElementById("save-btn");
    var cancelBtn = document.getElementById("cancel-btn");
    var editorToolbar = document.getElementById("editor-toolbar");
    var article = document.querySelector("article");
    var editorEl = document.getElementById("editor-container");
    var mdPath = editBtn ? editBtn.getAttribute("data-md-path") : null;
    var vditor = null;
    var originalHTML = article ? article.innerHTML : "";

    if (!editBtn || !article || !editorEl || !mdPath) return;

    function loadCSS(href) {
        if (document.querySelector('link[href="' + href + '"]')) return;
        var link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = href;
        document.head.appendChild(link);
    }

    function loadScript(src, cb) {
        if (window.Vditor) return cb();
        var s = document.createElement("script");
        s.src = src;
        s.onload = cb;
        s.onerror = function () {
            alert("Failed to load editor. Check your internet connection.");
        };
        document.head.appendChild(s);
    }

    function enterEditMode() {
        editBtn.style.display = "none";
        editorToolbar.style.display = "flex";
        article.style.display = "none";
        editorEl.style.display = "block";

        // Fetch raw markdown
        var root = window.rootPath || "";
        var xhr = new XMLHttpRequest();
        xhr.open("GET", root + "api/page?path=" + encodeURIComponent(mdPath));
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
        loadCSS(VDITOR_CDN + "/dist/index.css");
        loadScript(VDITOR_CDN + "/dist/index.min.js", function () {
            vditor = new Vditor("editor-container", {
                mode: "ir",
                value: markdown,
                height: "calc(100vh - 120px)",
                cdn: VDITOR_CDN,
                cache: { enable: false },
                toolbar: [
                    "headings", "bold", "italic", "strike", "link", "|",
                    "list", "ordered-list", "check", "quote", "|",
                    "code", "inline-code", "table", "line", "|",
                    "undo", "redo", "|",
                    "fullscreen", "outline"
                ],
                outline: { enable: true, position: "right" },
                counter: { enable: true },
                placeholder: "Start writing...",
                after: function () {
                    vditor.focus();
                }
            });
        });
    }

    function exitEditMode() {
        editBtn.style.display = "";
        editorToolbar.style.display = "none";
        article.style.display = "";
        editorEl.style.display = "none";

        if (vditor) {
            vditor.destroy();
            vditor = null;
        }
        editorEl.innerHTML = "";
    }

    function saveDocument() {
        if (!vditor) return;

        var content = vditor.getValue();
        saveBtn.disabled = true;
        saveBtn.textContent = "Saving...";

        var root = window.rootPath || "";
        var xhr = new XMLHttpRequest();
        xhr.open("PUT", root + "api/page?path=" + encodeURIComponent(mdPath));
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

    editBtn.addEventListener("click", enterEditMode);
    cancelBtn.addEventListener("click", exitEditMode);
    saveBtn.addEventListener("click", saveDocument);

    // Ctrl/Cmd+S to save while editing
    document.addEventListener("keydown", function (e) {
        if ((e.ctrlKey || e.metaKey) && e.key === "s" && vditor) {
            e.preventDefault();
            saveDocument();
        }
    });
})();
