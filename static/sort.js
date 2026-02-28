(function() {
    var btns = document.querySelectorAll('.sort-btn');
    var list = document.getElementById('page-list');
    if (!list) return;
    btns.forEach(function(btn) {
        btn.addEventListener('click', function() {
            btns.forEach(function(b) { b.classList.remove('sort-active'); });
            btn.classList.add('sort-active');
            var items = Array.from(list.children);
            var key = btn.getAttribute('data-sort');
            items.sort(function(a, b) {
                if (key === 'date') {
                    return b.getAttribute('data-date').localeCompare(a.getAttribute('data-date'));
                }
                return a.getAttribute('data-title').localeCompare(b.getAttribute('data-title'));
            });
            items.forEach(function(li) { list.appendChild(li); });
        });
    });
})();
