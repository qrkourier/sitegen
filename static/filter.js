(function () {
    "use strict";

    var index = null;
    var input = document.getElementById("sidebar-filter");
    if (!input) return;

    function canonicalize(href) {
        return href.replace(/^(\.\.\/)+/, "");
    }

    function loadIndex(cb) {
        if (index) return cb();
        var root = window.rootPath || "";
        var xhr = new XMLHttpRequest();
        xhr.open("GET", root + "search-index.json");
        xhr.onload = function () {
            if (xhr.status === 200) {
                try { index = JSON.parse(xhr.responseText); } catch (e) { index = []; }
            } else {
                index = [];
            }
            cb();
        };
        xhr.onerror = function () { index = []; cb(); };
        xhr.send();
    }

    // ---- Age filter state ----

    var ageSelect = document.getElementById("age-select");
    var AGE_KEY = "sitegen-age-filter";

    function parseHours(val) {
        var h = parseInt(val, 10);
        return isNaN(h) || h <= 0 ? 0 : h;
    }

    function getAgeCutoff() {
        if (!ageSelect) return 0;
        var hours = parseHours(ageSelect.value);
        return hours > 0 ? Date.now() - hours * 3600000 : 0;
    }

    function isAged(el) {
        var cutoff = getAgeCutoff();
        if (cutoff === 0) return false;
        var dateStr = el.getAttribute("data-updated") || "";
        if (!dateStr) return false;
        var ts = new Date(dateStr + "T00:00:00").getTime();
        return ts < cutoff;
    }

    // ---- Combined filter (text + age) ----

    function applyFilters() {
        var tree = document.querySelector(".tree");
        if (!tree) return;

        var query = input.value.trim();
        var cutoff = getAgeCutoff();

        // Build text match set from index
        var textMatches = null; // null = no text filter active
        if (query) {
            textMatches = {};
            var q = query.toLowerCase();
            var escaped = q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
            var wordRe = new RegExp("\\b" + escaped + "\\b", "i");
            if (index) {
                index.forEach(function (entry) {
                    // Skip index entries outside the age filter window —
                    // avoids expensive regex on irrelevant old planfiles
                    if (cutoff > 0 && entry.u) {
                        var ts = new Date(entry.u + "T00:00:00").getTime();
                        if (ts < cutoff) return;
                    }
                    if (entry.t.toLowerCase().indexOf(q) !== -1 ||
                        wordRe.test(entry.c)) {
                        textMatches[entry.p] = true;
                    }
                });
            }
        }

        // Apply to each file item: must pass both age and text filters
        tree.querySelectorAll(".tree-file").forEach(function (el) {
            // Age check
            var dateStr = el.getAttribute("data-updated") || "";
            var aged = false;
            if (cutoff > 0 && dateStr) {
                var ts = new Date(dateStr + "T00:00:00").getTime();
                aged = ts < cutoff;
            }
            if (aged) {
                el.classList.add("tree-file-aged");
            } else {
                el.classList.remove("tree-file-aged");
            }

            // Text check (only for non-aged items)
            if (aged) {
                el.style.display = "";
                return;
            }

            if (!textMatches) {
                // No text filter — show all non-aged items
                el.style.display = "";
                return;
            }

            var link = el.querySelector("a");
            if (!link) { el.style.display = "none"; return; }
            var path = canonicalize(link.getAttribute("href"));
            var title = link.textContent.toLowerCase();
            var q = query.toLowerCase();
            var visible = textMatches[path] || title.indexOf(q) !== -1;
            el.style.display = visible ? "" : "none";
        });

        // Update directory visibility
        tree.querySelectorAll(".tree-dir").forEach(function (el) {
            var hasVisible = el.querySelector(
                ".tree-file:not(.tree-file-aged):not([style*='display: none'])"
            );
            // A dir is aged if all children are aged
            var hasNonAged = el.querySelector(".tree-file:not(.tree-file-aged)");
            if (hasNonAged) {
                el.classList.remove("tree-dir-aged");
            } else {
                el.classList.add("tree-dir-aged");
            }
            // Show/hide based on text filter within non-aged items
            if (textMatches) {
                el.style.display = hasVisible ? "" : "none";
                var details = el.querySelector("details");
                if (details) {
                    if (hasVisible) {
                        details.setAttribute("open", "");
                    } else {
                        details.removeAttribute("open");
                    }
                }
            } else {
                el.style.display = "";
                var details = el.querySelector("details");
                if (details) details.removeAttribute("open");
            }
        });
    }

    // ---- Event wiring ----

    var FILTER_KEY = "sitegen-sidebar-filter";

    input.addEventListener("input", function () {
        var q = input.value.trim();
        try { localStorage.setItem(FILTER_KEY, q); } catch (e) {}
        loadIndex(function () { applyFilters(); });
    });

    // Restore saved text filter
    try {
        var savedFilter = localStorage.getItem(FILTER_KEY);
        if (savedFilter) {
            input.value = savedFilter;
        }
    } catch (e) {}

    if (ageSelect) {
        // Restore saved age preference
        try {
            var saved = localStorage.getItem(AGE_KEY);
            if (saved) {
                ageSelect.value = saved;
            }
        } catch (e) {}

        ageSelect.addEventListener("change", function () {
            try { localStorage.setItem(AGE_KEY, ageSelect.value); } catch (e) {}
            applyFilters();
        });
    }

    // Initial apply (after index loads, so text filter works on first load)
    loadIndex(function () { applyFilters(); });
})();
