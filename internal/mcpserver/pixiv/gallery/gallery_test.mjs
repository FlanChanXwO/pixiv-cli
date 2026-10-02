import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

// Local bridge/DOM fixture: no network, host credentials, browser installation or dependency.
const html = await readFile(new URL('./gallery.html', import.meta.url), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
class Element {
  constructor(tag) { this.tag = tag; this.children = []; this.events = {}; this.textContent = ''; this.src = ''; }
  append(child) { this.children.push(child); }
  replaceChildren() { this.children = []; }
  addEventListener(name, callback) { this.events[name] = callback; }
  removeAttribute(name) { this[name] = ''; }
}
const status = new Element('p'), cards = new Element('main');
const listeners = {}, sent = [], created = [], revoked = [], intersections = [], resizes = [];
const parent = {postMessage(message, origin) { sent.push({message: JSON.parse(JSON.stringify(message)), origin}); }};
const window = {parent, addEventListener(name, callback) { (listeners[name] ??= []).push(callback); }};
class Observer {
  constructor(callback, list) { this.callback = callback; this.targets = []; this.disconnected = false; list.push(this); }
  observe(target) { this.targets.push(target); }
  unobserve(target) { this.targets = this.targets.filter(item => item !== target); }
  disconnect() { this.disconnected = true; this.targets = []; }
}
vm.runInNewContext(script, {window, document: {body: new Element('body'), documentElement: {scrollWidth: 640, scrollHeight: 400}, getElementById: id => id === 'status' ? status : cards, createElement: tag => new Element(tag)},
  URL: {createObjectURL(blob) { const url = 'blob:fixture-' + created.length; created.push({url, blob}); return url; }, revokeObjectURL: url => revoked.push(url)}, Blob, atob,
  IntersectionObserver: class extends Observer { constructor(callback) { super(callback, intersections); } },
  ResizeObserver: class extends Observer { constructor(callback) { super(callback, resizes); } }
});
const flush = () => new Promise(resolve => setImmediate(resolve));
const receive = (data, source = parent, origin = 'https://host.test') => listeners.message.forEach(fn => fn({source, origin, data: {jsonrpc:'2.0', ...data}}));
const latest = method => sent.filter(item => item.message.method === method).at(-1).message;
const initialize = latest('ui/initialize');
assert.equal(initialize.params.appInfo.name, 'pixiv-gallery');
assert.equal(initialize.params.protocolVersion, '2026-01-26');
receive({id: initialize.id, result: {}}, {});
await flush();
assert.equal(sent.length, 1, 'foreign window cannot initialize');
receive({id: initialize.id, result: {protocolVersion:'2026-01-26', hostCapabilities: {serverTools:{}}, hostContext:{}}});
await flush();
assert.ok(latest('ui/notifications/initialized'));
const record = {id:'42', type:'illust', title:'<img src=x onerror=alert(1)>', user:{name:'artist'}, tags:[{name:'<script>'}]};
const show = () => receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[record]},content:[]}});
show();
assert.equal(cards.children.length, 1);
assert.equal(cards.children[0].children[0].textContent, record.title);
assert.equal(sent.filter(x => x.message.method === 'tools/call').length, 0, 'offscreen cards do not fetch');
let observer = intersections.at(-1);
observer.callback([{isIntersecting:true, target:cards.children[0]}]);
const stale = latest('tools/call');
assert.deepEqual(stale.params, {name:'pixiv_artwork_media',arguments:{illust_id:42,pages:[1],quality:'thumbnail'}});
receive({method:'ui/notifications/tool-input',params:{arguments:{word:'new'}}});
assert.equal(latest('notifications/cancelled').params.requestId, stale.id);
receive({id: stale.id, result:{}});
await flush();
assert.equal(created.length, 0, 'obsolete responses cannot create blobs');
show();
observer = intersections.at(-1);
observer.callback([{isIntersecting:true, target:cards.children[0]}]);
const call = latest('tools/call');
const data = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==';
const bytes = Buffer.from(data,'base64');
receive({id:call.id,result:{content:[{type:'image',mimeType:'image/png',data}],structuredContent:{complete:true,pages:[{page:1,content_index:0,mime_type:'image/png',size:bytes.length}]}}});
await flush();
assert.equal(created.length, 1);
assert.deepEqual(Buffer.from(await created[0].blob.arrayBuffer()), bytes);
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[]}}}, parent, 'https://other.test');
assert.equal(cards.children.length, 1, 'changed origin rejected');
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[null, {type:'illust',id:'invalid',title:'bad-id'}]}}});
assert.equal(cards.children.length, 2, 'malformed records remain explicit, not silently dropped');
receive({method:'ui/resource-teardown',id:99,params:{}});
assert.deepEqual(revoked, [created[0].url]);
assert.equal(cards.children.length, 0);
assert.ok(resizes.every(item => item.disconnected), 'teardown must stop size observers');
assert.ok(sent.some(item => item.message.id === 99 && item.message.result));
console.log('Gallery bridge mock: initialize, source/origin checks, text rendering, lazy bytes, stale view cancellation, blob and observer cleanup passed');
