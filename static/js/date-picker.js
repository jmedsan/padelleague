(function() {
    var isDesktop = window.matchMedia('(hover: hover) and (pointer: fine)').matches;
    if (!isDesktop) return;

    document.querySelectorAll('.date-calendar-dropdown').forEach(function(dd) {
        dd.classList.remove('hidden');
    });

    function syncCalendarFromInput(wrap) {
        var input = wrap.querySelector('input[type="date"]');
        var cal = wrap.querySelector('calendar-date');
        if (!input || !cal) return;
        if (input.value) {
            cal.setAttribute('value', input.value);
        }
        if (input.min) cal.setAttribute('min', input.min);
        if (input.max) cal.setAttribute('max', input.max);
    }

    function init() {
        document.querySelectorAll('.date-picker-wrap').forEach(syncCalendarFromInput);
    }

    document.addEventListener('change', function(e) {
        if (!e.target.matches('.date-picker-wrap input[type="date"]')) return;
        var wrap = e.target.closest('.date-picker-wrap');
        if (wrap) syncCalendarFromInput(wrap);
    });

    document.addEventListener('change', function(e) {
        if (!e.target.matches('calendar-date')) return;
        var wrap = e.target.closest('.date-picker-wrap');
        if (!wrap) return;
        var input = wrap.querySelector('input[type="date"]');
        if (!input) return;
        input.value = e.target.value;
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));

        var dd = wrap.querySelector('.dropdown-content');
        if (dd) dd.blur();
        var label = wrap.querySelector('label[tabindex]');
        if (label) label.blur();
    });

    init();
    document.body.addEventListener('htmx:afterSettle', init);
})();
