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

    function filter(query) {
        var tree = document.querySelector(".tree");
        if (!tree) return;

        if (!query) {
            tree.querySelectorAll(".tree-file, .tree-dir").forEach(function (el) {
                el.style.display = "";
            });
            tree.querySelectorAll("details").forEach(function (d) {
                d.removeAttribute("open");
            });
            return;
        }

        var q = query.toLowerCase();
        var matches = {};
        if (index) {
            index.forEach(function (entry) {
                if (entry.t.toLowerCase().indexOf(q) !== -1 ||
                    entry.c.toLowerCase().indexOf(q) !== -1) {
                    matches[entry.p] = true;
                }
            });
        }

        tree.querySelectorAll(".tree-file").forEach(function (el) {
            var link = el.querySelector("a");
            if (!link) { el.style.display = "none"; return; }
            var path = canonicalize(link.getAttribute("href"));
            var title = link.textContent.toLowerCase();
            var visible = matches[path] || title.indexOf(q) !== -1;
            el.style.display = visible ? "" : "none";
        });

        tree.querySelectorAll(".tree-dir").forEach(function (el) {
            var hasVisible = el.querySelector(".tree-file:not([style*='display: none'])");
            el.style.display = hasVisible ? "" : "none";
            var details = el.querySelector("details");
            if (details) {
                if (hasVisible) {
                    details.setAttribute("open", "");
                } else {
                    details.removeAttribute("open");
                }
            }
        });
    }

    input.addEventListener("input", function () {
        var q = input.value.trim();
        loadIndex(function () { filter(q); });
    });
})();
