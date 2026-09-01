(function() {
    'use strict';

    function root(el) { return el.closest('.score-input'); }

    // isValidSet — padel set rules: winner needs 6+ games, win by 2 up to
    // 6-4, or 7-5, or a 7-6 tiebreak. No ties, no scores above 7.
    function isValidSet(a, b) {
        if (a === b) return false;
        var hi = Math.max(a, b), lo = Math.min(a, b);
        if (hi === 6 && lo <= 4) return true;
        if (hi === 7 && (lo === 5 || lo === 6)) return true;
        return false;
    }

    function setPair(si, i) {
        return { a: si.querySelector('[name=s'+i+'a]'), b: si.querySelector('[name=s'+i+'b]') };
    }

    // checkSet — mark the set's dropdowns invalid (red border) once both are
    // filled but don't form a legal padel set score.
    function checkSet(si, i) {
        var p = setPair(si, i);
        if (!p.a || !p.b) return null;
        if (p.a.value === '' || p.b.value === '') {
            p.a.classList.remove('select-error');
            p.b.classList.remove('select-error');
            return null;
        }
        var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);
        var ok = isValidSet(a, b);
        p.a.classList.toggle('select-error', !ok);
        p.b.classList.toggle('select-error', !ok);
        return ok ? (a > b ? 1 : 2) : null;
    }

    function checkThirdSet(si) {
        var w1 = checkSet(si, 1);
        var w2 = checkSet(si, 2);
        var show = w1 !== null && w2 !== null && w1 !== w2;
        var group = si.querySelector('.score-set-group[data-set="3"]');
        if (group) group.classList.toggle('hidden', !show);
        if (!show) {
            var p3 = setPair(si, 3);
            if (p3.a) { p3.a.value = ''; p3.a.classList.remove('select-error'); }
            if (p3.b) { p3.b.value = ''; p3.b.classList.remove('select-error'); }
        } else {
            checkSet(si, 3);
        }
    }

    // matchWinner — 1 or 2 once that side has won 2 valid sets, else 0.
    function matchWinner(si) {
        var won1 = 0, won2 = 0;
        for (var i = 1; i <= 3; i++) {
            var w = checkSet(si, i);
            if (w === 1) won1++;
            else if (w === 2) won2++;
        }
        if (won1 >= 2) return 1;
        if (won2 >= 2) return 2;
        return 0;
    }

    function updateWinner(si) {
        var el = si.querySelector('.score-winner');
        if (!el) return;
        var w = matchWinner(si);
        if (w === 1) el.textContent = (si.getAttribute('data-pair1') || 'Pareja 1') + ' gana';
        else if (w === 2) el.textContent = (si.getAttribute('data-pair2') || 'Pareja 2') + ' gana';
        else el.textContent = '';
    }

    function compose(si) {
        var parts = [];
        for (var i = 1; i <= 3; i++) {
            var p = setPair(si, i);
            if (p.a && p.b && p.a.value !== '' && p.b.value !== '') {
                parts.push(p.a.value + '-' + p.b.value);
            }
        }
        var hidden = si.querySelector('.score-composed');
        if (hidden) hidden.value = parts.join(' ');
    }

    // isComplete — every visible set is filled and valid, and exactly one
    // side has reached 2 sets won (a well-formed match result).
    function isComplete(si) {
        var group3 = si.querySelector('.score-set-group[data-set="3"]');
        var set3Visible = group3 && !group3.classList.contains('hidden');
        var w1 = checkSet(si, 1), w2 = checkSet(si, 2);
        if (w1 === null || w2 === null) return false;
        if (set3Visible) {
            var w3 = checkSet(si, 3);
            if (w3 === null) return false;
        }
        return matchWinner(si) !== 0;
    }

    // updateSubmitState — disables the form's submit button while the score
    // is partially entered but not a complete valid result. A fully empty
    // score is left enabled: some forms (admin override) allow submitting
    // with no score change at all.
    function updateSubmitState(si) {
        var form = si.closest('form');
        if (!form) return;
        var btn = form.querySelector('button[type=submit]');
        if (!btn) return;
        var anyFilled = si.querySelectorAll('select.score-cell').length &&
            Array.prototype.some.call(si.querySelectorAll('select.score-cell'), function(s) {
                return !s.closest('.hidden') && s.value !== '';
            });
        btn.disabled = anyFilled && !isComplete(si);
    }

    function refresh(si) {
        checkThirdSet(si);
        updateWinner(si);
        compose(si);
        updateSubmitState(si);
    }

    window.fillCells = function(scoreEl, scoreStr) {
        var si = scoreEl.closest ? scoreEl.closest('.score-input') : scoreEl;
        if (!si) return;
        var sets = (scoreStr || '').trim().split(/\s+/);
        for (var i = 0; i < 3; i++) {
            var p = setPair(si, i + 1);
            if (p.a && p.b) {
                if (sets[i]) {
                    var parts = sets[i].split('-');
                    p.a.value = parts[0] || '';
                    p.b.value = parts[1] || '';
                } else {
                    p.a.value = '';
                    p.b.value = '';
                }
            }
        }
        refresh(si);
    };

    // fillNearestScore — from a clickable result box, find the score-input in the
    // enclosing form/section and fill it with the box's score. Lets the whole box
    // be the fill affordance without being inside the score-input.
    window.fillNearestScore = function(el, scoreStr) {
        var scope = el.closest('form') || el.closest('.dispute-resolve') || document;
        var si = scope.querySelector('.score-input');
        if (si) window.fillCells(si, scoreStr);
    };

    document.addEventListener('change', function(e) {
        if (!e.target.classList.contains('score-cell')) return;
        var si = root(e.target);
        if (!si) return;
        refresh(si);
    });

    document.addEventListener('submit', function(e) {
        var si = e.target.querySelector('.score-input');
        if (!si) return;
        compose(si);
    });

    function hydrateAll(container) {
        var els = (container || document).querySelectorAll('.score-input');
        for (var i = 0; i < els.length; i++) {
            var v = els[i].getAttribute('data-value');
            if (v) fillCells(els[i], v);
            else refresh(els[i]);
        }
    }

    document.addEventListener('DOMContentLoaded', function() { hydrateAll(); });
    document.addEventListener('htmx:afterSettle', function(e) { hydrateAll(e.detail.elt); });

    document.addEventListener('htmx:configRequest', function(e) {
        var form = e.detail.elt.closest('form');
        if (!form) return;
        var si = form.querySelector('.score-input');
        if (!si) return;
        compose(si);
        var hidden = si.querySelector('.score-composed');
        if (hidden) e.detail.parameters[hidden.name] = hidden.value;
    });
})();
