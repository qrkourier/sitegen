(function () {
    "use strict";

    function copyText(text, btn) {
        function showToast(ok) {
            var t = document.createElement("span");
            t.className = "badge-copy-toast";
            t.textContent = ok ? "Copied!" : "Failed";
            btn.closest(".badge").appendChild(t);
            setTimeout(function () { t.remove(); }, 1200);
        }
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).then(
                function () { showToast(true); },
                function () { fallback(text, showToast); }
            );
        } else {
            fallback(text, showToast);
        }
    }

    function fallback(text, showToast) {
        var ta = document.createElement("textarea");
        ta.value = text;
        ta.style.cssText = "position:fixed;left:-9999px";
        document.body.appendChild(ta);
        ta.select();
        try { var ok = document.execCommand("copy"); showToast(ok); }
        catch (e) { showToast(false); }
        ta.remove();
    }

    document.addEventListener("click", function (e) {
        var btn = e.target.closest(".badge-copy");
        if (!btn) return;
        e.preventDefault();
        e.stopPropagation();
        copyText(btn.getAttribute("data-copy") || "", btn);
    });
})();
