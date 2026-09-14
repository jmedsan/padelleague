(function () {
  'use strict';

  var MS_PER_DAY = 86400000;
  var fmt = new Intl.DateTimeFormat('es-ES', { hour: '2-digit', minute: '2-digit', hour12: false });

  function startOfDay(d) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  }

  function localRelDate(iso) {
    var d = new Date(iso);
    if (isNaN(d)) return '';

    var now = new Date();
    var todayStart = startOfDay(now);
    var targetStart = startOfDay(d);
    var days = Math.round((targetStart - todayStart) / MS_PER_DAY);

    var hasTime = d.getHours() !== 0 || d.getMinutes() !== 0;
    var timePart = hasTime ? ' ' + fmt.format(d) : '';

    if (days === 0) return 'hoy' + timePart;
    if (days === 1) return 'mañana';
    if (days === -1) return 'ayer' + timePart;
    if (days > 1 && days <= 7) return 'en ' + days + ' días';
    if (days < -1 && days >= -7) return 'hace ' + (-days) + ' días';

    var dd = String(d.getDate()).padStart(2, '0');
    var mm = String(d.getMonth() + 1).padStart(2, '0');
    var yyyy = d.getFullYear();
    return dd + '/' + mm + '/' + yyyy + (hasTime ? ' ' + fmt.format(d) : '');
  }

  function localize(root) {
    var els = (root || document).querySelectorAll('time[datetime]:not([data-localized])');
    els.forEach(function (el) {
      var iso = el.getAttribute('datetime');
      if (!iso) return;
      var text = localRelDate(iso);
      if (text) {
        el.textContent = text;
        el.title = new Date(iso).toLocaleString('es-ES');
      }
      el.dataset.localized = '1';
    });
  }

  document.addEventListener('DOMContentLoaded', function () { localize(); });
  document.body.addEventListener('htmx:afterSettle', function (e) {
    localize(e.detail.elt);
  });
})();
