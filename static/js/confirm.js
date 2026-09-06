// Replaces htmx's native window.confirm() for hx-confirm with the DaisyUI
// modal in layout.html (#confirm-modal). Optional per-element attributes:
//   data-confirm-title  — modal heading (default "Confirmar")
//   data-confirm-ok     — OK button label (default "Confirmar")
//   data-confirm-danger — style the OK button as btn-error
// On cancel, a bubbling `confirm:cancelled` event fires on the element so
// templates can undo optimistic UI (e.g. re-flip a toggle).
(function () {
    var dlg = document.getElementById('confirm-modal');
    if (!dlg) return;
    var msg = document.getElementById('confirm-message');
    var title = document.getElementById('confirm-title');
    var ok = document.getElementById('confirm-ok');
    var pending = null;

    function settle(accepted) {
        var p = pending;
        pending = null;
        if (dlg.open) dlg.close();
        if (!p) return;
        if (accepted) {
            p.issueRequest(true);
        } else {
            p.elt.dispatchEvent(new CustomEvent('confirm:cancelled', { bubbles: true }));
        }
    }

    document.body.addEventListener('htmx:confirm', function (evt) {
        if (!evt.detail.question) return; // element has no hx-confirm
        evt.preventDefault();
        pending = evt.detail;
        var el = evt.detail.elt;
        msg.textContent = evt.detail.question;
        title.textContent = el.getAttribute('data-confirm-title') || 'Confirmar';
        ok.textContent = el.getAttribute('data-confirm-ok') || 'Confirmar';
        ok.className = 'btn ' + (el.hasAttribute('data-confirm-danger') ? 'btn-error' : 'btn-primary');
        dlg.showModal();
        ok.focus();
    });

    dlg.querySelectorAll('[data-confirm]').forEach(function (btn) {
        btn.addEventListener('click', function () { settle(btn.dataset.confirm === 'ok'); });
    });

    // Esc key or backdrop click closes the dialog natively; treat as cancel.
    dlg.addEventListener('close', function () { if (pending) settle(false); });
})();
