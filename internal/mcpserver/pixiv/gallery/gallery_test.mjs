import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

// Local bridge/DOM fixture: no network, host credentials, browser installation or dependency.
const downloads = !process.argv.includes('--without-download');
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
const rootElement = {scrollWidth:640,scrollHeight:400,style:{}};
const listeners = {}, sent = [], created = [], revoked = [], intersections = [], resizes = [];
const parent = {postMessage(message, origin) { sent.push({message: JSON.parse(JSON.stringify(message)), origin}); }};
const window = {parent, addEventListener(name, callback) { (listeners[name] ??= []).push(callback); }};
class Observer {
  constructor(callback, list) { this.callback = callback; this.targets = []; this.disconnected = false; list.push(this); }
  observe(target) { this.targets.push(target); }
  unobserve(target) { this.targets = this.targets.filter(item => item !== target); }
  disconnect() { this.disconnected = true; this.targets = []; }
}
vm.runInNewContext(script, {window, document: {body: new Element('body'), documentElement: rootElement, getElementById: id => id === 'status' ? status : cards, createElement: tag => new Element(tag)},
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
receive({id: initialize.id, result: {protocolVersion:'2026-01-26', hostCapabilities: {serverTools:{}, ...(downloads ? {downloadFile:{}} : {})}, hostContext:{}}});
await flush();
assert.ok(latest('ui/notifications/initialized'));
receive({method:'ui/notifications/host-context-changed',params:{theme:'dark',containerDimensions:{width:500,height:260}}});
assert.equal(rootElement.style.colorScheme,'dark');
assert.equal(rootElement.style.height,'260px');
resizes[0].callback();
assert.deepEqual(latest('ui/notifications/size-changed').params,{width:500,height:260});
receive({method:'ui/notifications/host-context-changed',params:{theme:'light',containerDimensions:{maxWidth:420,maxHeight:200}}});
assert.equal(rootElement.style.height,'');
assert.equal(rootElement.style.maxHeight,'200px');
resizes[0].callback();
assert.deepEqual(latest('ui/notifications/size-changed').params,{width:420,height:200});

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
const staticDownload = button('Download 42_p3.png');
assert.ok(staticDownload);
if (downloads) {
  staticDownload.events.click();
  const staticRequest = latest('ui/download-file');
  assert.equal(staticRequest.params.contents[0].resource.blob,data);
  assert.equal(staticRequest.params.contents[0].resource.mimeType,'image/png');
  receive({id:staticRequest.id,result:{}});
  await flush();
} else assert.equal(staticDownload.disabled,true);
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

receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{...record,type:'ugoira'}]},content:[]}});
button('Open artwork').events.click();
const animationDetail = latest('tools/call');
receive({id:animationDetail.id,result:{structuredContent:{records:[{...record,type:'ugoira',page_count:1}]},content:[]}});
await flush();
assert.ok(button('Load animation'), 'missing full animation control');
button('Load animation').events.click();
const animationCall = latest('tools/call');
assert.deepEqual(animationCall.params.arguments,{illust_id:42,animation_format:'gif'});
const animationData = Buffer.from('GIF89a-fixture-binary').toString('base64');
const animationContent = {type:'resource',resource:{uri:'urn:sha256:fixture',mimeType:'image/gif',blob:animationData}};
receive({id:animationCall.id,result:{content:[animationContent,{type:'image',mimeType:'image/png',data}],structuredContent:{complete:true,pages:[{page:1,content_index:0,preview_content_index:1,filename:'42.gif',mime_type:'image/gif',size:Buffer.from(animationData,'base64').length}],failures:[]}}});
await flush();
assert.ok(button('Play animation'));
const animatedImage = elements(cards).find(node => node.tag === 'img');
const firstFrameURL = animatedImage.src;
button('Play animation').events.click();
assert.notEqual(animatedImage.src,firstFrameURL,'must display full animation, not just preview');
button('Show preview').events.click();
assert.equal(animatedImage.src,firstFrameURL);
const downloadButton = button('Download 42.gif');
assert.ok(downloadButton);
if (downloads) {
  downloadButton.events.click();
  const downloadCall = latest('ui/download-file');
  assert.deepEqual(downloadCall.params,{contents:[animationContent]});
  receive({id:downloadCall.id,result:{isError:true}});
  await flush();
  assert.ok(elements(cards).some(node => node.textContent.includes('Download not confirmed')));
  downloadButton.events.click();
  const secondDownload = latest('ui/download-file');
  receive({id:secondDownload.id,result:{}});
  await flush();
  assert.ok(elements(cards).some(node => node.textContent.includes('Host confirmed download')));
} else {
  assert.equal(downloadButton.disabled,true);
  const before = sent.length;
  downloadButton.events.click();
  assert.equal(sent.length,before,'missing capability must not send download requests');
}
const animationFormat = elements(cards).find(node => node.tag === 'select');
animationFormat.value = 'apng';
button('Load animation').events.click();
const apngCall = latest('tools/call');
assert.deepEqual(apngCall.params.arguments,{illust_id:42,animation_format:'apng'});
receive({id:apngCall.id,result:{content:[],structuredContent:{complete:false,pages:[],failures:[{page:1,error:'media_read_failed'}]},isError:true}});
await flush();
assert.ok(elements(cards).some(node => node.textContent.includes('Animation read failed or incomplete')));
button('Load animation').events.click();
const apngRetry = latest('tools/call');
// The VM checks wire routing; real GIF/APNG encoding is covered by the Go media fixture.
receive({id:apngRetry.id,result:{content:[{type:'resource',resource:{uri:'urn:sha256:apng-fixture',mimeType:'image/apng',blob:data}},{type:'image',mimeType:'image/png',data}],structuredContent:{complete:true,pages:[{page:1,content_index:0,preview_content_index:1,filename:'42.apng',mime_type:'image/apng',size:bytes.length}]}}});
await flush();
assert.ok(button('Download 42.apng'));

// Continue the host-provided logical page without guessing a cursor or dropping records.
receive({method:'ui/notifications/host-context-changed',params:{toolInfo:{tool:{name:'pixiv_search_illust'}}}});
receive({method:'ui/notifications/tool-input',params:{arguments:{word:'cat',limit:2,sort:'date_desc'}}});
const pageMeta = (page, more) => ({page,limit:2,returned:2,has_more:more,next_page:more?page+1:null});
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[record,{...record,id:'43'}],pagination:pageMeta(1,true)}}});
assert.ok(button('Load next page'),'missing discovery continuation');
button('Load next page').events.click();
const continuation = latest('tools/call');
assert.deepEqual(continuation.params,{name:'pixiv_search_illust',arguments:{word:'cat',limit:2,sort:'date_desc',page:2}});
receive({id:continuation.id,result:{structuredContent:{records:[{...record,id:'44'},{...record,id:'45'}],pagination:pageMeta(2,false)}}});
await flush();
assert.equal(cards.children.filter(node=>node.tag==='article').length,4,'continuation must append every record');
assert.equal(button('Load next page'),undefined);


receive({method:'ui/notifications/host-context-changed',params:{toolInfo:{tool:{name:'pixiv_recommended'}}}});
receive({method:'ui/notifications/tool-input',params:{arguments:{kind:'all',limit:2,illust_filter:{type:'manga'},novel_filter:{min_views:3},user_filter:{id:8}}}});
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{...record,type:'manga'},{type:'novel',id:'9',title:'Novel'},{type:'user',id:'8'}],pagination:{illust:pageMeta(1,true),manga:pageMeta(1,true),novel:pageMeta(1,true),user:pageMeta(1,false)}}}});
assert.ok(button('Load next manga page'),'missing mixed stream continuation');
assert.equal(button('Load next illust page'),undefined,'manga-only filter cannot continue an illust stream');
button('Load next manga page').events.click();
const mangaNext = latest('tools/call');
assert.deepEqual(mangaNext.params.arguments,{kind:'manga',limit:2,page:2,illust_filter:{type:'manga'}});
receive({id:mangaNext.id,result:{isError:true,structuredContent:{records:[]}}});
await flush();
assert.equal(cards.children.filter(node=>node.tag==='article').length,3,'failed page preserves all streams');
button('Load next manga page').events.click();
receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,id:'46',type:'manga'}],pagination:{manga:pageMeta(2,false)}}}});
await flush();
assert.equal(cards.children.filter(node=>node.tag==='article').length,4);
assert.ok(button('Load next novel page'),'other stream continuation must survive');
button('Load next novel page').events.click();
const novelNext = latest('tools/call');
assert.deepEqual(novelNext.params.arguments,{kind:'novel',limit:2,page:2,novel_filter:{min_views:3}});
receive({method:'ui/notifications/tool-input',params:{arguments:{kind:'novel',limit:2}}});
receive({id:novelNext.id,result:{structuredContent:{records:[{type:'novel',id:'late'}],pagination:{novel:pageMeta(2,false)}}}});
await flush();
assert.equal(cards.children.length,0,'old continuation cannot replace a new view');


for (const [name,args] of [['pixiv_illust_related',{illust_id:42}],['pixiv_illust_ranking',{mode:'day'}],['pixiv_illust_recommended',{}],['pixiv_search_illust',{word:'cats'}]]) {
receive({method:'ui/notifications/host-context-changed',params:{toolInfo:{name:'ignored',tool:{name}}}});
receive({method:'ui/notifications/tool-input',params:{arguments:args}});
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[record],pagination:{page:1,limit:null,has_more:true,next_page:null,next_cursor:'opaque-fixture'}}}});
assert.ok(button('Load next page'),'missing default batch cursor control');
button('Load next page').events.click();
const cursorCall = latest('tools/call');
assert.deepEqual(cursorCall.params,{name,arguments:{...args,cursor:'opaque-fixture'}});
receive({id:cursorCall.id,result:{structuredContent:{records:[{...record,id:'99'}],pagination:{page:1,limit:null,has_more:false,next_page:null}}}});
await flush();
assert.equal(cards.children.filter(node=>node.tag==='article').length,2);
assert.equal(button('Load next page'),undefined);

}
receive({method:'ui/notifications/host-context-changed',params:{toolInfo:{tool:{name:'pixiv_search_illust'}}}});
receive({method:'ui/notifications/tool-input',params:{arguments:{word:'unbounded'}}});
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[record],pagination:{page:1,limit:null,returned:1,has_more:true,next_page:null}}}});
assert.equal(button('Load next page'),undefined,'must not invent a page size or cursor');
assert.ok(elements(cards).some(node=>node.textContent.includes('no usable continuation')));

receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[null, {type:'illust',id:'invalid',title:'bad-id'}]}}});
assert.equal(cards.children.length, 2, 'malformed records remain explicit, not silently dropped');
receive({method:'ui/resource-teardown',id:99,params:{}});
assert.deepEqual(new Set(revoked), new Set(created.map(item => item.url)));
assert.equal(cards.children.length, 0);
assert.ok(resizes.every(item => item.disconnected), 'teardown must stop size observers');
assert.ok(sent.some(item => item.message.id === 99 && item.message.result));
console.log('Gallery bridge mock: security/context, pages/partial, bookmarks, GIF/APNG routing, host downloads and cleanup passed');
