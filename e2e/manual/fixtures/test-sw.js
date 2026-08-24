// Minimal SW for push API only — no fetch handler, no clients.claim().
// The page is NOT controlled, so the in-browser fetch mock intercepts requests.
self.addEventListener('push', function(event) {
    var data = event.data ? event.data.json() : {};
    event.waitUntil(
        self.registration.showNotification(data.title || 'Test', {
            body: data.body || '',
            data: { url: data.url || '/' }
        })
    );
});
