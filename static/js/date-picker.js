(function() {
    if (!window.matchMedia('(hover: hover) and (pointer: fine)').matches) return;

    function initWrap(wrap) {
        if (wrap.dataset.dpInit) return;
        wrap.dataset.dpInit = '1';

        var dd = wrap.querySelector('.date-calendar-dropdown');
        if (dd) dd.classList.remove('hidden');

        var input = wrap.querySelector('input[type="date"]');
        var cal = wrap.querySelector('calendar-date');
        if (!input || !cal) return;

        if (input.value) cal.setAttribute('value', input.value);
        if (input.min) cal.setAttribute('min', input.min);
        if (input.max) cal.setAttribute('max', input.max);

        input.addEventListener('change', function() {
            cal.setAttribute('value', input.value);
        });

        cal.addEventListener('change', function() {
            input.value = cal.value;
            input.dispatchEvent(new Event('input', { bubbles: true }));
            input.dispatchEvent(new Event('change', { bubbles: true }));
            var label = wrap.querySelector('label[tabindex]');
            if (label) label.blur();
        });
    }

    function initAll() {
        document.querySelectorAll('.date-picker-wrap').forEach(initWrap);
    }

    var observer = new MutationObserver(function(mutations) {
        for (var i = 0; i < mutations.length; i++) {
            for (var j = 0; j < mutations[i].addedNodes.length; j++) {
                var node = mutations[i].addedNodes[j];
                if (node.nodeType !== 1) continue;
                if (node.classList && node.classList.contains('date-picker-wrap')) {
                    initWrap(node);
                } else if (node.querySelectorAll) {
                    node.querySelectorAll('.date-picker-wrap').forEach(initWrap);
                }
            }
        }
    });

    observer.observe(document.body, { childList: true, subtree: true });
    initAll();
})();
