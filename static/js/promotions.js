// Promociones: remembers dismissed offers in localStorage and removes the card
// when the player closes it or opens the offer. Storage errors never break the page.
(function () {
  var KEY = 'promo-dismissed';

  function read() {
    try {
      var list = JSON.parse(localStorage.getItem(KEY));
      return Array.isArray(list) ? list : [];
    } catch (e) {
      return [];
    }
  }

  window.promoDismissed = function () {
    return read().join(',');
  };

  window.promoDismiss = function (id) {
    var list = read();
    if (list.indexOf(id) === -1) list.push(id);
    try {
      localStorage.setItem(KEY, JSON.stringify(list));
    } catch (e) {
      // Blocked storage: the card is still removed for this page.
    }
  };

  function handle(ev) {
    if (!ev.target.closest) return;
    var el = ev.target.closest('[data-promo-link], [data-promo-dismiss]');
    if (!el) return;
    var card = el.closest('[data-promo-card]');
    window.promoDismiss(el.getAttribute('data-promo-link') || el.getAttribute('data-promo-dismiss'));
    if (card) card.remove();
  }

  document.addEventListener('click', handle);
  document.addEventListener('auxclick', function (ev) {
    if (ev.button === 1) handle(ev);
  });
})();
