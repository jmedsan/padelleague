function urlBase64ToUint8Array(base64String) {
    var padding = '='.repeat((4 - base64String.length % 4) % 4);
    var base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
    var raw = atob(base64);
    var arr = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
    return arr;
}

(function() {
    var toggle = document.getElementById('push-toggle');
    if (!toggle || !window.VAPID_PUBLIC_KEY) return;

    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
        toggle.closest('.form-control').style.display = 'none';
        return;
    }

    navigator.serviceWorker.ready.then(function(reg) {
        reg.pushManager.getSubscription().then(function(sub) {
            toggle.checked = !!sub;
        });
    });

    toggle.addEventListener('change', function() {
        if (this.checked) {
            Notification.requestPermission().then(function(perm) {
                if (perm !== 'granted') {
                    toggle.checked = false;
                    return;
                }
                navigator.serviceWorker.ready.then(function(reg) {
                    reg.pushManager.subscribe({
                        userVisibleOnly: true,
                        applicationServerKey: urlBase64ToUint8Array(window.VAPID_PUBLIC_KEY)
                    }).then(function(sub) {
                        fetch('/push/subscribe', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify(sub.toJSON())
                        });
                    }).catch(function() {
                        toggle.checked = false;
                    });
                });
            });
        } else {
            navigator.serviceWorker.ready.then(function(reg) {
                reg.pushManager.getSubscription().then(function(sub) {
                    if (sub) {
                        fetch('/push/unsubscribe', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ endpoint: sub.endpoint })
                        }).then(function() {
                            sub.unsubscribe();
                        });
                    }
                });
            });
        }
    });
})();
