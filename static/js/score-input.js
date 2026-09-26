(function() {
    'use strict';

    function root(el) { return el.closest('.score-input'); }

    function isValidSet(a, b) {
        if (a === b) return false;
        var hi = Math.max(a, b), lo = Math.min(a, b);
        if (hi === 6 && lo <= 4) return true;
        if (hi === 7 && (lo === 5 || lo === 6)) return true;
        return false;
    }

    function isOpenSet(a, b) {
        return a >= 0 && a <= 6 && b >= 0 && b <= 6 && !isValidSet(a, b);
    }

    function setPair(si, i) {
        return { a: si.querySelector('[name=s'+i+'a]'), b: si.querySelector('[name=s'+i+'b]') };
    }

    function isLocked(si, i) {
        var group = si.querySelector('.score-set-group[data-set="'+i+'"]');
        return group && group.hasAttribute('data-locked');
    }

    function isLastVisibleSet(si, i) {
        for (var j = i + 1; j <= 3; j++) {
            var group = si.querySelector('.score-set-group[data-set="'+j+'"]');
            if (group && !group.classList.contains('hidden')) return false;
        }
        return true;
    }

    // checkSet returns: 1 (pair1 won), 2 (pair2 won), 0 (open/unfinished), null (invalid/empty)
    function checkSet(si, i) {
        var p = setPair(si, i);
        if (!p.a || !p.b) return null;
        if (isLocked(si, i)) {
            var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);
            if (isNaN(a) || isNaN(b)) return null;
            return a > b ? 1 : (b > a ? 2 : null);
        }
        if (p.a.value === '' || p.b.value === '') {
            p.a.classList.remove('select-error');
            p.b.classList.remove('select-error');
            return null;
        }
        var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);

        // Last visible set may be an open (unfinished) set — always auto-detect
        if (isLastVisibleSet(si, i)) {
            var ok = isValidSet(a, b) || isOpenSet(a, b);
            p.a.classList.toggle('select-error', !ok);
            p.b.classList.toggle('select-error', !ok);
            if (!ok) return null;
            if (isOpenSet(a, b)) return 0; // in progress — no winner yet
            return a > b ? 1 : (b > a ? 2 : 0);
        }

        var ok = isValidSet(a, b);
        p.a.classList.toggle('select-error', !ok);
        p.b.classList.toggle('select-error', !ok);
        return ok ? (a > b ? 1 : 2) : null;
    }

    function checkThirdSet(si) {
        var w1 = checkSet(si, 1);
        var w2 = checkSet(si, 2);
        var show = w1 !== null && w2 !== null && w1 !== w2 && w1 !== 0 && w2 !== 0;
        var group = si.querySelector('.score-set-group[data-set="3"]');
        if (group && !isLocked(si, 3)) {
            group.classList.toggle('hidden', !show);
            if (!show) {
                var p3 = setPair(si, 3);
                if (p3.a) { p3.a.value = ''; p3.a.classList.remove('select-error'); }
                if (p3.b) { p3.b.value = ''; p3.b.classList.remove('select-error'); }
            } else {
                checkSet(si, 3);
            }
        }
    }

    function countCompletedSets(si) {
        var won1 = 0, won2 = 0;
        for (var i = 1; i <= 3; i++) {
            var p = setPair(si, i);
            if (!p.a || !p.b || p.a.value === '' || p.b.value === '') continue;
            var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);
            if (isValidSet(a, b)) {
                if (a > b) won1++; else won2++;
            }
        }
        return { won1: won1, won2: won2 };
    }

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

    function getOpenSetValues(si) {
        for (var i = 3; i >= 1; i--) {
            var group = si.querySelector('.score-set-group[data-set="'+i+'"]');
            if (!group || group.classList.contains('hidden') || isLocked(si, i)) continue;
            var p = setPair(si, i);
            if (!p.a || !p.b || p.a.value === '' || p.b.value === '') continue;
            var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);
            if (!isValidSet(a, b) && isOpenSet(a, b) && isLastVisibleSet(si, i)) {
                return { a: a, b: b, set: i };
            }
        }
        return null;
    }

    function getCarriedString(si) {
        var parts = [];
        for (var i = 1; i <= 3; i++) {
            if (!isLocked(si, i)) continue;
            var p = setPair(si, i);
            if (p.a && p.b && p.a.value !== '' && p.b.value !== '') {
                parts.push(p.a.value + '-' + p.b.value);
            }
        }
        // Add completed non-locked sets
        for (var i = 1; i <= 3; i++) {
            if (isLocked(si, i)) continue;
            var group = si.querySelector('.score-set-group[data-set="'+i+'"]');
            if (!group || group.classList.contains('hidden')) continue;
            var p = setPair(si, i);
            if (!p.a || !p.b || p.a.value === '' || p.b.value === '') continue;
            var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);
            if (isValidSet(a, b)) {
                parts.push(a + '-' + b);
            }
        }
        return parts.join(' ');
    }

    function updateWinner(si) {
        var el = si.querySelector('.score-winner');
        if (!el) return;
        var w = matchWinner(si);

        if (w === 1) {
            el.textContent = (si.getAttribute('data-pair1') || 'Pareja 1') + ' gana';
            el.className = 'text-sm font-medium text-success score-winner';
            return;
        }
        if (w === 2) {
            el.textContent = (si.getAttribute('data-pair2') || 'Pareja 2') + ' gana';
            el.className = 'text-sm font-medium text-success score-winner';
            return;
        }

        // Mirror EvaluateScore: check rule win (3-game lead + already won 1 set)
        var open = getOpenSetValues(si);
        var completed = countCompletedSets(si);
        if (open) {
            var diff = open.a - open.b;
            if (diff >= 3 && completed.won1 >= 1) {
                el.textContent = (si.getAttribute('data-pair1') || 'Pareja 1') + ' gana por la regla de los 3 juegos';
                el.className = 'text-sm font-medium text-success score-winner';
                return;
            }
            if (diff <= -3 && completed.won2 >= 1) {
                el.textContent = (si.getAttribute('data-pair2') || 'Pareja 2') + ' gana por la regla de los 3 juegos';
                el.className = 'text-sm font-medium text-success score-winner';
                return;
            }

            // No rule win — show "No terminado" with resume info
            var carried = getCarriedString(si);
            if (carried) {
                el.textContent = 'No terminado · se reanuda desde ' + carried + ' 0-0';
            } else {
                el.textContent = 'No terminado';
            }
            el.className = 'text-sm font-medium text-warning score-winner';
            return;
        }

        // isComplete but no winner: e.g. set 1 = 6-4, set 2 untouched (open)
        if (isComplete(si)) {
            var carried = getCarriedString(si);
            if (carried) {
                el.textContent = 'No terminado · se reanuda desde ' + carried + ' 0-0';
            } else {
                el.textContent = 'No terminado';
            }
            el.className = 'text-sm font-medium text-warning score-winner';
            return;
        }

        el.textContent = '';
        el.className = 'text-sm font-medium text-success score-winner';
    }

    function compose(si) {
        var parts = [];
        for (var i = 1; i <= 3; i++) {
            if (isLocked(si, i)) continue;
            var p = setPair(si, i);
            if (p.a && p.b && p.a.value !== '' && p.b.value !== '') {
                parts.push(p.a.value + '-' + p.b.value);
            }
        }
        var hidden = si.querySelector('.score-composed');
        if (hidden) hidden.value = parts.join(' ');
    }

    function isComplete(si) {
        // Check for a 2-set winner first (normal complete match)
        if (matchWinner(si) !== 0) return true;

        // Auto-detect unfinished: at least one non-locked set filled,
        // all non-last visible sets are valid, last visible set is an open set
        var anyFilled = false;
        for (var i = 1; i <= 3; i++) {
            if (isLocked(si, i)) continue;
            var group = si.querySelector('.score-set-group[data-set="'+i+'"]');
            if (!group || group.classList.contains('hidden')) continue;
            var p = setPair(si, i);
            if (!p.a || !p.b || p.a.value === '' || p.b.value === '') continue;
            anyFilled = true;
            var a = parseInt(p.a.value, 10), b = parseInt(p.b.value, 10);
            if (!isLastVisibleSet(si, i)) {
                if (!isValidSet(a, b)) return false;
            } else {
                if (!isValidSet(a, b) && !isOpenSet(a, b)) return false;
                if (a === 0 && b === 0) return false;
            }
        }
        return anyFilled;
    }

    function updateSubmitState(si) {
        var form = si.closest('form');
        if (!form) return;
        var btn = form.querySelector('button[type=submit]');
        if (!btn) return;
        var anyFilled = si.querySelectorAll('select.score-cell').length &&
            Array.prototype.some.call(si.querySelectorAll('select.score-cell'), function(s) {
                return !s.closest('.hidden') && !s.disabled && s.value !== '';
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
            if (isLocked(si, i + 1)) continue;
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

    window.fillNearestScore = function(el, scoreStr) {
        var scope = el.closest('form') || el.closest('.dispute-resolve') || document;
        var si = scope.querySelector('.score-input');
        if (si) window.fillCells(si, scoreStr);
    };

    document.addEventListener('change', function(e) {
        if (e.target.classList.contains('score-cell')) {
            var si = root(e.target);
            if (!si) return;
            refresh(si);
        }
    });

    document.addEventListener('submit', function(e) {
        var si = e.target.querySelector('.score-input');
        if (!si) return;
        compose(si);
    });

    function hydrateAll(container) {
        var els = (container || document).querySelectorAll('.score-input');
        for (var i = 0; i < els.length; i++) {
            var si = els[i];
            // Lock carried sets
            var carried = si.getAttribute('data-carried');
            if (carried) {
                var carriedSets = carried.trim().split(/\s+/);
                for (var j = 0; j < carriedSets.length && j < 3; j++) {
                    var group = si.querySelector('.score-set-group[data-set="'+(j+1)+'"]');
                    if (!group) continue;
                    group.setAttribute('data-locked', '');
                    var parts = carriedSets[j].split('-');
                    var p = setPair(si, j + 1);
                    if (p.a) { p.a.value = parts[0] || ''; p.a.disabled = true; p.a.classList.add('opacity-50'); }
                    if (p.b) { p.b.value = parts[1] || ''; p.b.disabled = true; p.b.classList.add('opacity-50'); }
                    var label = group.querySelector('.text-xs');
                    if (label) label.innerHTML = '🔒 S' + (j+1);
                }
            }
            var v = si.getAttribute('data-value');
            if (v) fillCells(si, v);
            else refresh(si);
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
