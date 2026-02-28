(function() {
    var saved = localStorage.getItem('sitegen-theme');
    if (saved) {
        document.documentElement.setAttribute('data-theme', saved);
    }
    document.addEventListener('DOMContentLoaded', function() {
        var sel = document.getElementById('theme-select');
        if (!sel) return;
        if (saved) sel.value = saved;
        sel.addEventListener('change', function() {
            var theme = sel.value;
            if (theme === 'light') {
                document.documentElement.removeAttribute('data-theme');
            } else {
                document.documentElement.setAttribute('data-theme', theme);
            }
            localStorage.setItem('sitegen-theme', theme);
        });
    });
})();
