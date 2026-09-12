// Narrow, repeatable v1 → text generation cleanup. Auth, avatars and unrelated
// applications are deliberately outside this migration's ownership.
export async function retireLegacyWebCache({base, caches, navigator}) {
  const root = new URL(base);
  const obsolete = new Set([
    'main.dart.js', 'flutter_bootstrap.js', 'flutter.js', 'flutter_service_worker.js',
    'assets/config.json', 'assets/assets/config.json',
  ].map(path => new URL(path, root).pathname));
  if (navigator?.serviceWorker?.getRegistrations) {
    for (const registration of await navigator.serviceWorker.getRegistrations()) {
      const worker = registration.active ?? registration.waiting ?? registration.installing;
      if (registration.scope === root.href && worker?.scriptURL &&
          new URL(worker.scriptURL).origin === root.origin &&
          new URL(worker.scriptURL).pathname === new URL('flutter_service_worker.js', root).pathname) {
        await registration.unregister();
      }
    }
  }
  if (!caches?.keys) return;
  for (const name of await caches.keys()) {
    if (!['flutter-app-cache', 'flutter-temp-cache'].includes(name)) continue;
    const cache = await caches.open(name);
    for (const request of await cache.keys()) {
      const url = new URL(request.url);
      if (url.origin === root.origin && obsolete.has(url.pathname)) await cache.delete(request);
    }
  }
}
