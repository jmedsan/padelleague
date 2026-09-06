(function() {
    function urlBase64ToUint8Array(base64String) {
        var padding = '='.repeat((4 - base64String.length % 4) % 4);
        var base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
        var raw = atob(base64);
        var arr = new Uint8Array(raw.length);
        for (var i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
        return arr;
    }

    var toggle = document.getElementById('push-toggle');
    if (!toggle || !window.VAPID_PUBLIC_KEY) return;

    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
        toggle.closest('.form-control').style.display = 'none';
        return;
    }

    function showPushError(msg) {
        var existing = document.getElementById('push-error-toast');
        if (existing) existing.remove();
        var toast = document.createElement('div');
        toast.id = 'push-error-toast';
        toast.className = 'toast toast-center z-50';
        toast.innerHTML = '<div class="alert alert-error">' + msg + '</div>';
        document.body.appendChild(toast);
        setTimeout(function() { toast.remove(); }, 3000);
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
                    if (perm === 'denied') {
                        showPushError('Las notificaciones están bloqueadas en el navegador. Actívalas en los ajustes del sitio para continuar.');
                    }
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
                        }).then(function(response) {
                            if (!response.ok) {
                                sub.unsubscribe();
                                toggle.checked = false;
                                showPushError('No se pudieron activar las notificaciones. Inténtalo de nuevo.');
                            }
                        }).catch(function() {
                            sub.unsubscribe();
                            toggle.checked = false;
                            showPushError('No se pudieron activar las notificaciones. Inténtalo de nuevo.');
                        });
                    }).catch(function() {
                        toggle.checked = false;
                        showPushError('No se pudieron activar las notificaciones. Inténtalo de nuevo.');
                    });
                });
            });
        } else {
            navigator.serviceWorker.ready.then(function(reg) {
                reg.pushManager.getSubscription().then(function(sub) {
                    if (sub) {
                        sub.unsubscribe();
                        fetch('/push/unsubscribe', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ endpoint: sub.endpoint })
                        }).then(function(response) {
                            if (!response.ok) {
                                showPushError('Las notificaciones se desactivaron localmente, pero el servidor no respondió.');
                            }
                        }).catch(function() {
                            showPushError('Las notificaciones se desactivaron localmente, pero el servidor no respondió.');
                        });
                    }
                });
            });
        }
    });
})();
