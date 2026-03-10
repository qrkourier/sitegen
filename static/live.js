(function () {
    "use strict";

    var toast = null;
    var es = null;
    var RECONNECT_MS = 3000;

    function connect() {
        es = new EventSource("/api/events");
        es.onmessage = function (e) {
            if (e.data === "changed") {
                showToast();
            }
        };
        es.onerror = function () {
            es.close();
            setTimeout(connect, RECONNECT_MS);
        };
    }

    function showToast() {
        if (toast) return;
        toast = document.createElement("div");
        toast.className = "live-toast";
        toast.innerHTML =
            '<span class="live-toast-dot"></span>' +
            '<span class="live-toast-msg">Source changed</span>' +
            '<button class="live-toast-reload">Refresh</button>' +
            '<button class="live-toast-close" aria-label="Dismiss">&times;</button>';
        toast.querySelector(".live-toast-reload").addEventListener("click", function () {
            softReload();
        });
        toast.querySelector(".live-toast-close").addEventListener("click", function () {
            dismiss();
        });
        document.body.appendChild(toast);
        requestAnimationFrame(function () {
            toast.classList.add("live-toast-visible");
        });
    }

    function dismiss() {
        if (!toast) return;
        toast.classList.remove("live-toast-visible");
        var el = toast;
        toast = null;
        setTimeout(function () { el.remove(); }, 300);
    }

    function softReload() {
        var xhr = new XMLHttpRequest();
        xhr.open("GET", window.location.href);
        xhr.responseType = "document";
        xhr.onload = function () {
            if (xhr.status !== 200) {
                window.location.reload();
                return;
            }
            var newDoc = xhr.response;

            // Swap article content
            var oldArticle = document.querySelector("article");
            var newArticle = newDoc.querySelector("article");
            if (oldArticle && newArticle) {
                oldArticle.innerHTML = newArticle.innerHTML;
            }

            // Swap sidebar tree
            var oldTree = document.querySelector(".tree");
            var newTree = newDoc.querySelector(".tree");
            if (oldTree && newTree) {
                oldTree.innerHTML = newTree.innerHTML;
            }

            // Swap TOC
            var oldToc = document.querySelector(".toc");
            var newToc = newDoc.querySelector(".toc");
            if (oldToc && newToc) {
                oldToc.innerHTML = newToc.innerHTML;
            }

            // Swap page-meta-header badges
            var oldMeta = document.querySelector(".page-meta-header");
            var newMeta = newDoc.querySelector(".page-meta-header");
            if (oldMeta && newMeta) {
                oldMeta.innerHTML = newMeta.innerHTML;
            } else if (!oldMeta && newMeta) {
                var content = document.querySelector(".content");
                if (content) content.insertBefore(newMeta, content.firstChild);
            } else if (oldMeta && !newMeta) {
                oldMeta.remove();
            }

            // Update title
            var newTitle = newDoc.querySelector("title");
            if (newTitle) document.title = newTitle.textContent;

            dismiss();
        };
        xhr.onerror = function () {
            window.location.reload();
        };
        xhr.send();
    }

    connect();
})();
