var CACHE_NAME = 'padelleague-v2';
var OFFLINE_URL = '/static/offline.html';
var ASSETS = [
    '/static/css/styles.css',
    '/static/js/htmx.min.js',
    '/static/js/score-input.js',
    '/static/img/icon-192.png',
    '/static/img/icon-512.png',
    OFFLINE_URL,
];

self.addEventListener('install', function(event) {
    event.waitUntil(
        caches.open(CACHE_NAME).then(function(cache) {
            return cache.addAll(ASSETS);
        })
    );
    self.skipWaiting();
});

self.addEventListener('activate', function(event) {
    event.waitUntil(
        caches.keys().then(function(names) {
            return Promise.all(
                names.filter(function(n) { return n !== CACHE_NAME; })
                     .map(function(n) { return caches.delete(n); })
            );
        })
    );
    self.clients.claim();
});

self.addEventListener('fetch', function(event) {
    if (event.request.mode === 'navigate') {
        event.respondWith(
            fetch(event.request).catch(function() {
                return caches.match(OFFLINE_URL);
            })
        );
        return;
    }
    if (event.request.url.includes('/static/')) {
        event.respondWith(
            caches.match(event.request).then(function(cached) {
                var fetched = fetch(event.request).then(function(response) {
                    caches.open(CACHE_NAME).then(function(cache) {
                        cache.put(event.request, response.clone());
                    });
                    return response;
                }).catch(function(err) {
                    if (cached) return cached;
                    throw err;
                });
                return cached || fetched;
            })
        );
    }
});

self.addEventListener('push', function(event) {
    var data = event.data ? event.data.json() : {};
    event.waitUntil(
        self.registration.showNotification(data.title || 'Dale Fuerte a la Bola', {
            body: data.body || '',
            icon: '/static/img/icon-192.png',
            badge: '/static/img/icon-192.png',
            data: { url: data.url || '/' }
        })
    );
});

self.addEventListener('notificationclick', function(event) {
    event.notification.close();
    var url = event.notification.data.url || '/';
    event.waitUntil(
        self.clients.matchAll({ type: 'window' }).then(function(list) {
            for (var i = 0; i < list.length; i++) {
                if (list[i].url.indexOf(self.location.origin) !== -1) {
                    list[i].navigate(url);
                    return list[i].focus();
                }
            }
            return self.clients.openWindow(url);
        })
    );
});
