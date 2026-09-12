import test from 'node:test';
import assert from 'node:assert/strict';
import {retireLegacyWebCache} from '../../web/text_generation.mjs';

test('migration removes only obsolete Knowoff assets and its old worker; retry is safe',async()=>{
 const kept=new Set(['https://play.test/app/main.dart.js','https://play.test/app/flutter_bootstrap.js','https://play.test/app/assets/config.json','https://play.test/api/avatar/me','https://play.test/app/assets/avatars/owl.webp','https://play.test/other/main.dart.js']);
 const cache={keys:async()=>[...kept].map(url=>({url})),delete:async(request)=>kept.delete(request.url)};
 let removed=0;const own={scope:'https://play.test/app/',active:{scriptURL:'https://play.test/app/flutter_service_worker.js'},unregister:async()=>{removed++;}};
 const other={scope:'https://play.test/other/',active:{scriptURL:'https://play.test/other/flutter_service_worker.js'},unregister:async()=>assert.fail('unrelated worker removed')};
 const caches={keys:async()=>['flutter-app-cache','avatars'],open:async(name)=>{assert.equal(name,'flutter-app-cache');return cache;}};
 const navigator={serviceWorker:{getRegistrations:async()=>[own,other]}};
 await retireLegacyWebCache({base:'https://play.test/app/',caches,navigator});
 assert.equal(removed,1);assert.deepEqual([...kept],['https://play.test/api/avatar/me','https://play.test/app/assets/avatars/owl.webp','https://play.test/other/main.dart.js']);
 await retireLegacyWebCache({base:'https://play.test/app/',caches,navigator});assert.equal(kept.size,3);
});
test('missing browser cache APIs are a harmless no-op',async()=>{
 await retireLegacyWebCache({base:'https://play.test/app/',navigator:{}});
});
