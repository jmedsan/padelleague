(function() {
    'use strict';

    var CELLS = ['s1a','s1b','s2a','s2b','s3a','s3b'];
    var MAX_SET12 = 7, MAX_SET3 = 15;

    function root(el) { return el.closest('.score-input'); }

    function cellMax(cell) {
        return parseInt(cell.getAttribute('maxlength')) === 2 ? MAX_SET3 : MAX_SET12;
    }

    function checkRange(cell) {
        var v = cell.value;
        if (v === '') { cell.classList.remove('input-error'); return; }
        var n = parseInt(v);
        var ok = !isNaN(n) && n >= 0 && n <= cellMax(cell);
        cell.classList.toggle('input-error', !ok);
    }

    function checkThirdSet(si) {
        var s1a = si.querySelector('[name=s1a]');
        var s1b = si.querySelector('[name=s1b]');
        var s2a = si.querySelector('[name=s2a]');
        var s2b = si.querySelector('[name=s2b]');
        if (!s1a || !s1b || !s2a || !s2b) return;
        var show = false;
        if (s1a.value !== '' && s1b.value !== '' && s2a.value !== '' && s2b.value !== '') {
            var w1 = (parseInt(s1a.value) > parseInt(s1b.value) ? 1 : 0) +
                     (parseInt(s2a.value) > parseInt(s2b.value) ? 1 : 0);
            show = (w1 === 1);
        }
        var groups = si.querySelectorAll('.set3-group');
        for (var i = 0; i < groups.length; i++) {
            if (show) groups[i].classList.remove('hidden');
            else groups[i].classList.add('hidden');
        }
        if (!show) {
            var s3a = si.querySelector('[name=s3a]');
            var s3b = si.querySelector('[name=s3b]');
            if (s3a) { s3a.value = ''; s3a.classList.remove('input-error'); }
            if (s3b) { s3b.value = ''; s3b.classList.remove('input-error'); }
        }
    }

    function compose(si) {
        var parts = [];
        for (var i = 1; i <= 3; i++) {
            var a = si.querySelector('[name=s'+i+'a]');
            var b = si.querySelector('[name=s'+i+'b]');
            if (a && b && a.value !== '' && b.value !== '') {
                parts.push(a.value + '-' + b.value);
            }
        }
        var hidden = si.querySelector('.score-composed');
        if (hidden) hidden.value = parts.join(' ');
    }

    function nextCell(si, name) {
        var idx = CELLS.indexOf(name);
        if (idx < 0 || idx >= CELLS.length - 1) return null;
        var next = si.querySelector('[name='+CELLS[idx+1]+']');
        if (next && next.closest('.hidden')) return null;
        return next;
    }

    function prevCell(si, name) {
        var idx = CELLS.indexOf(name);
        if (idx <= 0) return null;
        var prev = si.querySelector('[name='+CELLS[idx-1]+']');
        if (prev && prev.closest('.hidden')) return null;
        return prev;
    }

    window.fillCells = function(scoreEl, scoreStr) {
        var si = scoreEl.closest ? scoreEl.closest('.score-input') : scoreEl;
        if (!si) return;
        var sets = (scoreStr || '').trim().split(/\s+/);
        for (var i = 0; i < 3; i++) {
            var a = si.querySelector('[name=s'+(i+1)+'a]');
            var b = si.querySelector('[name=s'+(i+1)+'b]');
            if (a && b) {
                if (sets[i]) {
                    var parts = sets[i].split('-');
                    a.value = parts[0] || '';
                    b.value = parts[1] || '';
                } else {
                    a.value = '';
                    b.value = '';
                }
                checkRange(a);
                checkRange(b);
            }
        }
        checkThirdSet(si);
        compose(si);
    };

    // fillNearestScore — from a clickable result box, find the score-input in the
    // enclosing form/section and fill it with the box's score. Lets the whole box
    // be the fill affordance without being inside the score-input.
    window.fillNearestScore = function(el, scoreStr) {
        var scope = el.closest('form') || el.closest('.dispute-resolve') || document;
        var si = scope.querySelector('.score-input');
        if (si) window.fillCells(si, scoreStr);
    };

    document.addEventListener('input', function(e) {
        if (!e.target.classList.contains('score-cell')) return;
        var si = root(e.target);
        if (!si) return;
        e.target.value = e.target.value.replace(/[^0-9]/g, '');
        checkRange(e.target);
        checkThirdSet(si);
        var ml = parseInt(e.target.getAttribute('maxlength'));
        if (ml === 1 && e.target.value.length === 1) {
            var n = nextCell(si, e.target.name);
            if (n) n.focus();
        }
    });

    document.addEventListener('keydown', function(e) {
        if (e.key !== 'Backspace') return;
        if (!e.target.classList.contains('score-cell')) return;
        if (e.target.value !== '') return;
        var si = root(e.target);
        if (!si) return;
        var p = prevCell(si, e.target.name);
        if (p) { e.preventDefault(); p.focus(); p.select(); }
    });

    document.addEventListener('blur', function(e) {
        if (!e.target.classList.contains('score-cell')) return;
        checkRange(e.target);
    }, true);

    document.addEventListener('submit', function(e) {
        var si = e.target.querySelector('.score-input');
        if (!si) return;
        compose(si);
    });

    function hydrateAll(container) {
        var els = (container || document).querySelectorAll('.score-input[data-value]');
        for (var i = 0; i < els.length; i++) {
            var v = els[i].getAttribute('data-value');
            if (v) fillCells(els[i], v);
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
