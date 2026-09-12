{{flutter_js}}
{{flutter_build_config}}

(async () => {
  const {retireLegacyWebCache} = await import(new URL('text_generation.mjs?generation=2', document.baseURI));
  await retireLegacyWebCache({base: document.baseURI, caches: globalThis.caches, navigator: globalThis.navigator});
  // Bypass a surviving v1 controller and HTTP cache for the compiled entrypoint.
  // The v2 server also rejects old protocol clients before admission.
  for (const build of _flutter.buildConfig.builds) {
    if (build.mainJsPath) build.mainJsPath += '?text_generation=2';
  }
  await _flutter.loader.load();
})().catch(() => {
  // No private details in the upgrade failure surface; never start stale code.
  document.body.textContent = 'Knowoff needs a refresh to finish updating.';
});
