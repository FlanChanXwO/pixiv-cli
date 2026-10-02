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
  contains(node) { return this === node || this.children.some(child => child.contains(node)); }
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
const elements = node => [node, ...node.children.flatMap(elements)];
const button = label => elements(cards).find(node => node.tag === 'button' && node.textContent === label);
assert.ok(button('Open artwork'), 'missing artwork detail control');
button('Open artwork').events.click();
const detailCall = latest('tools/call');
assert.equal(detailCall.params.name, 'pixiv_illust_detail');
receive({id:detailCall.id,result:{structuredContent:{records:[{...record,page_count:3}]},content:[]}});
await flush();
const bookmarkCall = latest('tools/call');
assert.equal(bookmarkCall.params.name, 'pixiv_bookmark_detail');
receive({id:bookmarkCall.id,result:{structuredContent:{bookmarked:false,tags:null},content:[]}});
await flush();
const pageInput = elements(cards).find(node => node.tag === 'input');
assert.ok(pageInput);
pageInput.value = '3';
button('Load page').events.click();
const pageCall = latest('tools/call');
assert.deepEqual(pageCall.params.arguments,{illust_id:42,pages:[3],quality:'regular'});
receive({id:pageCall.id,result:{content:[{type:'image',mimeType:'image/png',data}],structuredContent:{complete:true,pages:[{page:3,content_index:0,mime_type:'image/png',size:bytes.length}],failures:[]}}});
await flush();
assert.ok(elements(cards).some(node => node.tag === 'img' && node.src.startsWith('blob:')));
const oldPageImage = elements(cards).find(node => node.tag === 'img');
const countBeforeInvalid = sent.length;
pageInput.value = '0';
button('Load page').events.click();
assert.equal(sent.length, countBeforeInvalid, 'out-of-range page must not call tools');
button('Load all pages').events.click();
const allPages = latest('tools/call');
assert.deepEqual(allPages.params.arguments,{illust_id:42,quality:'regular'}, 'all pages must omit pages rather than truncate');
receive({id:allPages.id,result:{isError:true,content:[{type:'image',mimeType:'image/png',data},{type:'image',mimeType:'image/png',data}],structuredContent:{complete:false,pages:[{page:1,content_index:0,mime_type:'image/png',size:bytes.length},{page:3,content_index:1,mime_type:'image/png',size:bytes.length}],failures:[{page:2,error:'upstream_error'}]}}});
await flush();
assert.equal(elements(cards).filter(node => node.tag === 'img').length,2);
assert.ok(elements(cards).some(node => node.textContent.includes('Undelivered pages: 2')));
oldPageImage.events.error();
assert.ok(elements(cards).some(node => node.textContent.includes('Undelivered pages: 2')), 'detached image errors must not replace current media status');
button('Bookmark private').events.click();
const mutation = latest('tools/call');
assert.deepEqual(mutation.params,{name:'pixiv_add_bookmark',arguments:{illust_id:42,restrict:'private',tags:[]}});
receive({id:mutation.id,result:{isError:true,structuredContent:{},content:[]}});
await flush();
assert.ok(elements(cards).some(node => node.textContent.includes('not confirmed')), 'failed mutation cannot claim success');
button('Refresh bookmark').events.click();
const refresh = latest('tools/call');
receive({id:refresh.id,result:{structuredContent:{bookmarked:true,restrict:'private',tags:['kept']},content:[]}});
await flush();
button('Bookmark public').events.click();
const update = latest('tools/call');
assert.deepEqual(update.params.arguments,{illust_id:42,restrict:'public',tags:['kept']});
receive({id:update.id,result:{isError:false,structuredContent:{},content:[]}});
await flush();
assert.equal(latest('tools/call').params.name,'pixiv_bookmark_detail', 'success must refresh state');
const confirmed = latest('tools/call');
receive({id:confirmed.id,result:{structuredContent:{bookmarked:true,restrict:'public',tags:['kept']},content:[]}});
await flush();
button('Remove bookmark').events.click();
const removal = latest('tools/call');
assert.deepEqual(removal.params,{name:'pixiv_remove_bookmark',arguments:{illust_id:42}});
receive({id:removal.id,result:{isError:false,content:[]}});
await flush();

receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[null, {type:'illust',id:'invalid',title:'bad-id'}]}}});
assert.equal(cards.children.length, 2, 'malformed records remain explicit, not silently dropped');
receive({method:'ui/resource-teardown',id:99,params:{}});
assert.deepEqual(new Set(revoked), new Set(created.map(item => item.url)));
assert.equal(cards.children.length, 0);
assert.ok(resizes.every(item => item.disconnected), 'teardown must stop size observers');
assert.ok(sent.some(item => item.message.id === 99 && item.message.result));
console.log('Gallery bridge mock: initialization/security, previews, detail pages/partial, explicit bookmarks, stale views and cleanup passed');
