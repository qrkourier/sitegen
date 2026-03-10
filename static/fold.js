(function () {
    "use strict";

    // Completion keywords — headings containing these (case-insensitive) are
    // folded by default. Matches whole words and the checkmark emoji.
    var COMPLETION_KEYWORDS = [
        "done", "completed", "complete", "finished", "resolved",
        "shipped", "merged", "superseded", "obsolete", "archived"
    ];
    var COMPLETION_RE = new RegExp(
        "(?:^|\\W)(?:" + COMPLETION_KEYWORDS.join("|") + ")(?:\\W|$)|\\u2705",
        "i"
    );

    var GLOBAL_KEY = "_global";

    var article = document.querySelector("article");
    if (!article) return;

    var topBar = document.getElementById("content-toolbar-top");
    var bottomBar = document.getElementById("content-toolbar-bottom");
    var pagePath = window.location.pathname;
    var foldState = {};

    // ---- Server-side fold state persistence ----

    function loadState(cb) {
        var xhr = new XMLHttpRequest();
        xhr.open("GET", "/api/folds?path=" + encodeURIComponent(pagePath));
        xhr.onload = function () {
            if (xhr.status === 200) {
                try { foldState = JSON.parse(xhr.responseText); } catch (e) {}
            }
            cb();
        };
        xhr.onerror = function () { cb(); };
        xhr.send();
    }

    function saveState() {
        var xhr = new XMLHttpRequest();
        xhr.open("PUT", "/api/folds?path=" + encodeURIComponent(pagePath));
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.send(JSON.stringify(foldState));
    }

    // ---- Helpers ----

    function hasIndividualState() {
        for (var k in foldState) {
            if (k !== GLOBAL_KEY) return true;
        }
        return false;
    }

    function isCompletionHeading(el) {
        return COMPLETION_RE.test(el.textContent);
    }

    function resolveOpen(id, isCompleted) {
        if (id in foldState) return foldState[id];
        if (GLOBAL_KEY in foldState) return foldState[GLOBAL_KEY];
        return !isCompleted;
    }

    // ---- Expand / Collapse / Reset ----

    function setAll(open) {
        if (hasIndividualState()) return;
        foldState[GLOBAL_KEY] = open;
        applyAll();
        saveState();
    }

    function applyAll() {
        article.querySelectorAll("details.section-fold").forEach(function (d) {
            var summary = d.querySelector("summary");
            if (!summary) return;
            var id = summary.id;
            var isCompleted = d.classList.contains("section-fold-completed");
            var isOpen = resolveOpen(id, isCompleted);
            if (isOpen) {
                d.setAttribute("open", "");
            } else {
                d.removeAttribute("open");
            }
        });
    }

    function resetAll() {
        foldState = {};
        applyAll();
        saveState();
    }

    // ---- Toolbar buttons ----

    function addFoldButtons(bar) {
        var expandBtn = document.createElement("button");
        expandBtn.className = "toolbar-btn";
        expandBtn.textContent = "Expand all";
        expandBtn.addEventListener("click", function () { setAll(true); });

        var collapseBtn = document.createElement("button");
        collapseBtn.className = "toolbar-btn";
        collapseBtn.textContent = "Collapse all";
        collapseBtn.addEventListener("click", function () { setAll(false); });

        var resetBtn = document.createElement("button");
        resetBtn.className = "toolbar-btn toolbar-btn-icon";
        resetBtn.title = "Reset folded state";
        resetBtn.setAttribute("aria-label", "Reset folded state");
        resetBtn.innerHTML = "&#x21bb;";
        resetBtn.addEventListener("click", resetAll);

        bar.appendChild(expandBtn);
        bar.appendChild(collapseBtn);
        bar.appendChild(resetBtn);
    }

    // ---- Section wrapping ----

    function wrapSections() {
        var headings = article.querySelectorAll("h2, h3");
        if (headings.length === 0) return;

        var hasFoldable = false;

        headings.forEach(function (heading) {
            var level = parseInt(heading.tagName.charAt(1), 10);
            var id = heading.id;
            if (!id) return;

            var contents = [];
            var next = heading.nextElementSibling;
            while (next) {
                var tag = next.tagName;
                if (/^H[1-6]$/.test(tag)) {
                    var nextLevel = parseInt(tag.charAt(1), 10);
                    if (nextLevel <= level) break;
                }
                contents.push(next);
                next = next.nextElementSibling;
            }

            if (contents.length === 0) return;
            hasFoldable = true;

            var isCompleted = isCompletionHeading(heading);
            var isOpen = resolveOpen(id, isCompleted);

            var details = document.createElement("details");
            details.className = "section-fold" + (isCompleted ? " section-fold-completed" : "");
            details.id = "fold-" + id;
            if (isOpen) details.setAttribute("open", "");

            var summary = document.createElement("summary");
            summary.className = "section-fold-summary section-fold-h" + level;

            while (heading.firstChild) {
                summary.appendChild(heading.firstChild);
            }
            summary.id = id;

            details.appendChild(summary);

            var frag = document.createDocumentFragment();
            contents.forEach(function (el) { frag.appendChild(el); });
            details.appendChild(frag);

            heading.parentNode.replaceChild(details, heading);

            details.addEventListener("toggle", function () {
                foldState[id] = details.open;
                saveState();
            });
        });

        if (hasFoldable) {
            if (topBar) addFoldButtons(topBar);
            if (bottomBar) addFoldButtons(bottomBar);
        }
    }

    loadState(wrapSections);
})();
