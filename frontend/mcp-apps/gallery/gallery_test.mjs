import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

// Local bridge/DOM fixture: no network, host credentials, browser installation or dependency.
const downloads = !process.argv.includes('--without-download');
const tools = !process.argv.includes('--without-tools');
const badProtocol = process.argv.includes('--bad-protocol');
const html = await readFile(new URL('./index.html', import.meta.url), 'utf8');
const script = await readFile(new URL('./gallery.js', import.meta.url), 'utf8');
class Element {
  constructor(tag) { this.tag = tag; this.children = []; this.events = {}; this.textContent = ''; this.src = ''; this.style = {}; this.scrollWidth = 180; this.clientWidth = 180; }
  append(child) { child.remove(); this.children.push(child); child.parentNode=this; }
  remove() { if(this.parentNode) { this.parentNode.children=this.parentNode.children.filter(n=>n!==this); this.parentNode=undefined; } }
  removeEventListener(name, callback) { if(this.events[name]===callback) delete this.events[name]; }
  replaceChildren(...children) { this.children = [...children]; }
  contains(node) { return this === node || this.children.some(child => child.contains(node)); }
  addEventListener(name, callback) { this.events[name] = callback; }
  showModal() { this.open=true; }
  close() { this.open=false; }
  click() { this.events.click?.(); }
  removeAttribute(name) { this[name] = ''; }
  setAttribute(name, value) { this[name] = String(value); }
  getAttribute(name) { return this[name]; }
}
const status = new Element('p'), cards = new Element('main');
const rootElement = {scrollWidth:640,scrollHeight:400,style:{}};
const body = new Element('body');body.scrollHeight=400;
const listeners = {}, sent = [], created = [], revoked = [], intersections = [], resizes = [];
const timers = new Map();let timerID=0;
const parent = {postMessage(message, origin) { sent.push({message: JSON.parse(JSON.stringify(message)), origin}); }};
const window = {parent, addEventListener(name, callback) { (listeners[name] ??= []).push(callback); }};
class Observer {
  constructor(callback, list) { this.callback = callback; this.targets = []; this.disconnected = false; list.push(this); }
  observe(target) { this.targets.push(target); }
  unobserve(target) { this.targets = this.targets.filter(item => item !== target); }
  disconnect() { this.disconnected = true; this.targets = []; }
}
vm.runInNewContext(script, {window, document: {body, documentElement: rootElement, getElementById: id => id === 'status' ? status : cards, createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag)},
  setTimeout(fn,ms) {const id=++timerID;timers.set(id,{fn,ms});return id;}, clearTimeout(id) {timers.delete(id);},
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
receive({id: initialize.id, result: {protocolVersion:badProtocol?'unsupported':'2026-01-26', hostCapabilities: {...(tools ? {serverTools:{}} : {}), ...(downloads ? {downloadFile:{}} : {})}, hostContext:{}}});
await flush();
if (badProtocol) {
  assert.ok(status.textContent.includes('Gallery bridge unavailable'));
  assert.equal(sent.filter(item=>item.message.method==='ui/notifications/initialized').length,0);
  receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{id:42,type:'illust'}]}}});
  assert.equal(cards.children.length,0,'uninitialized bridge must ignore results');
  assert.equal(sent.filter(item=>item.message.method==='tools/call').length,0);
  for (const dispose of listeners.pagehide) dispose();
  console.log('Gallery mock: unsupported protocol explicitly falls back without tool calls');
  process.exit(0);
}
assert.ok(latest('ui/notifications/initialized'));
assert.equal(status.textContent,'Waiting for tool results…');
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[]}}});
assert.equal(status.textContent,'No results.');
receive({method:'ui/notifications/tool-result',params:{isError:true,structuredContent:{records:[]}}});
assert.equal(status.textContent,'Tool failed or returned partial results.');
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

receive({method:'ui/notifications/host-context-changed',params:{containerDimensions:{}}});
body.scrollHeight=160;resizes[0].callback();assert.equal(latest('ui/notifications/size-changed').params.height,160,'content shrink should shrink requested host height');
body.scrollHeight=400;

const record = {id:'42',type:'illust',title:'<img src=x onerror=alert(1)>',caption:'<b>Description</b>',page_count:1,x_restrict:1,user:{name:'Artist <b>Name</b>'},published_at:'2026-10-01T12:00:00Z',total_views:420,total_bookmarks:0,width:640,height:800,tags:[{name:'tag'}]};
const elements = node => [node, ...node.children.flatMap(elements)];
const button = label => elements(cards).find(node => node.tag === 'button' && (node.getAttribute('aria-label') || node.textContent) === label);
const result = {structuredContent:{records:[record],media_origin:'https://media.test',images:[{illust_id:42,page:1,url:'https://media.test/media/signed.fixture'}]},content:[]};
assert.ok(!html.includes('<h1>Pixiv Gallery</h1>'));
receive({method:'ui/notifications/tool-result',params:result});
assert.equal(status.textContent,'','successful gallery has no result-count heading');
const card=cards.children[0]; assert.equal(card.children[0].className,'media-frame','image comes first');
assert.ok(!button('Open artwork'));assert.ok(!button('Load / retry preview'));
const retry=elements(card).find(n=>n.tag==='button' && n.textContent==='Retry image');assert.equal(retry.hidden,true);
assert.ok(!elements(card).some(n=>n.textContent===record.caption),'list keeps description in detail');
assert.ok(elements(card).some(n=>n.className==='artwork-author'&&n.textContent===record.user.name),'list shows author safely');
const rating=elements(card).find(n=>n.className==='rating-badge');assert.equal(rating?.textContent,'R-18');
assert.ok(!elements(card).some(n=>n.className==='page-badge'),'single page has no multi-image badge');
assert.equal(button(record.title).title,record.title,'full title remains available');
const tags=elements(card).find(n=>n.className==='tag-list');
assert.ok(tags,'tags have their own chip row');
assert.equal(tags.children[0].textContent,'tag');
const toggle=button('Show all tags');
assert.equal(toggle.hidden,true,'fitting tags do not need expansion');
tags.scrollWidth=700;tags.clientWidth=180;resizes[0].callback();
assert.equal(toggle.hidden,false,'overflow exposes expansion');
toggle.events.click();assert.equal(toggle.getAttribute('aria-expanded'),'true');
assert.equal(button('Show fewer tags'),toggle);
assert.equal(toggle.textContent,'...','tag expansion uses ellipsis');
assert.equal(toggle.title,'Show fewer tags','ellipsis retains its accessible hint');
toggle.events.click();assert.equal(toggle.getAttribute('aria-expanded'),'false');
tags.scrollWidth=180;tags.clientWidth=180;resizes[0].callback();assert.equal(toggle.hidden,true);
const tagGroup=elements(card).find(n=>n.className==='tags collapsed');
tagGroup.clientWidth=300;tags.scrollWidth=280;tags.clientWidth=250;resizes[0].callback();
assert.equal(toggle.hidden,true,'expansion button should disappear when tags fit without it');
assert.equal(sent.filter(x=>x.message.method==='tools/call').length,0);
intersections.at(-1).callback([{isIntersecting:true,target:card}]);await flush();
const img=elements(card).find(n=>n.tag==='img');assert.equal(img.src,'https://media.test/media/signed.fixture');assert.equal(img.referrerPolicy,'no-referrer');
img.events.load();assert.equal(retry.hidden,true);img.events.error();assert.equal(retry.hidden,false);
if(!tools){assert.equal(retry.disabled,true);for(const f of listeners.pagehide)f();console.log('Gallery mock: images without tool-call capability');process.exit(0)}
retry.events.click();const renewal=latest('tools/call');assert.deepEqual(renewal.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42],page:1,quality:'thumbnail'}});
receive({id:renewal.id,result});await flush();img.events.load();assert.equal(retry.hidden,true);
button(record.title).events.click();const detail=latest('tools/call');assert.equal(detail.params.name,'pixiv_illust_detail');receive({id:detail.id,result:{structuredContent:{records:[record]},content:[]}});await flush();
assert.equal(status.textContent,'','successful detail has no generic heading');
const detailSection=cards.children.find(n=>n.tag==='article');
assert.equal(detailSection.className,'artwork-detail','detail has its own responsive layout');
assert.ok(elements(detailSection).some(n=>n.className==='detail-media'));
assert.ok(elements(detailSection).some(n=>n.className==='detail-info'));
assert.ok(!elements(detailSection).some(n=>n.tag==='details'||n.tag==='summary'),'detail has no disclosure controls');
assert.ok(elements(detailSection).some(n=>n.className==='caption'&&n.textContent===record.caption),'description is directly visible as text');
assert.ok(elements(detailSection).some(n=>n.className==='artwork-author'&&n.textContent===record.user.name));
const metadata=elements(detailSection).filter(n=>n.className==='metadata-value').map(n=>n.textContent);
for(const value of ['420','0','42','640 × 800','1'])assert.ok(metadata.includes(value),'metadata includes '+value);
assert.ok(elements(detailSection).some(n=>n.tag==='time'&&n.dateTime===record.published_at));
assert.ok(elements(detailSection).some(n=>n.className==='rating-badge'&&n.textContent==='R-18'));
const back=button('Back to results');assert.ok(back.getAttribute('aria-label'));assert.equal(back.title,'Back to results');assert.ok(elements(back).some(n=>n.tag==='svg'&&n.getAttribute('aria-hidden')==='true'));
const bookmark=button('Bookmark'),remove=button('Remove bookmark'),refresh=button('Refresh bookmark');
const visibility=elements(cards).find(n=>n.tag==='select'&&n.getAttribute('aria-label')==='Bookmark visibility');
assert.ok(visibility);assert.equal(bookmark.disabled,true,'unknown state prohibits bookmark writes');assert.equal(remove.hidden,true);
const query=latest('tools/call');assert.equal(query.params.name,'pixiv_bookmark_detail');
receive({id:query.id,result:{structuredContent:{bookmarked:false,restrict:'public',tags:['keep']}}});await flush();
assert.equal(bookmark.disabled,false);assert.equal(remove.hidden,true);visibility.value='private';bookmark.events.click();
const add=latest('tools/call');assert.deepEqual(add.params,{name:'pixiv_add_bookmark',arguments:{illust_id:42,restrict:'private',tags:['keep']}});
assert.equal(bookmark.disabled,true);receive({id:add.id,result:{}});await flush();
const confirm=latest('tools/call');assert.equal(confirm.params.name,'pixiv_bookmark_detail');
receive({id:confirm.id,result:{structuredContent:{bookmarked:true,restrict:'private',tags:['keep']}}});await flush();
assert.ok(button('Save bookmark'));assert.equal(remove.hidden,false);assert.equal(visibility.value,'private');
remove.events.click();const removal=latest('tools/call');assert.equal(removal.params.name,'pixiv_remove_bookmark');
receive({id:removal.id,result:{isError:true}});await flush();assert.equal(bookmark.disabled,true,'unconfirmed write requires state refresh');
assert.ok(elements(cards).some(n=>n.textContent.includes('Bookmark change not confirmed')));
refresh.events.click();const recovery=latest('tools/call');receive({id:recovery.id,result:{structuredContent:{bookmarked:false,restrict:'public',tags:[]}}});await flush();assert.equal(bookmark.disabled,false);
assert.equal(button('Load page'),undefined,'single page has no page controls');assert.ok(!elements(cards).some(n=>n.tag==='input'&&n.type==='number'));
const detailImageCall=sent.filter(x=>x.message.method==='tools/call'&&x.message.params.name==='pixiv_show_gallery').at(-1).message;
receive({id:detailImageCall.id,result});await flush();assert.ok(elements(cards).some(n=>n.tag==='img'&&n.src==='https://media.test/media/signed.fixture'));
if(downloads){button('Download original page').events.click();const bytesCall=latest('tools/call');receive({id:bytesCall.id,result:{content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}],structuredContent:{pages:[{page:1,content_index:0}]}}});await flush();const save=latest('ui/download-file');receive({id:save.id,result:{isError:true}});await flush();assert.ok(elements(cards).some(n=>n.textContent==='Download failed. Retry explicitly.'));
button('Download original page').events.click();const secondBytes=latest('tools/call');receive({id:secondBytes.id,result:{content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}],structuredContent:{pages:[{page:1,content_index:0}]}}});await flush();
const acceptedDownload=latest('ui/download-file');receive({id:acceptedDownload.id,result:{}});await flush();
assert.ok(elements(cards).some(n=>n.textContent==='Download requested.'),'confirmed original download must not report failure');}
// Multipage detail keeps the selected page for preview and original download.
receive({method:'ui/notifications/tool-result',params:result});button(record.title).events.click();
const multipage=latest('tools/call');receive({id:multipage.id,result:{structuredContent:{records:[{...record,page_count:3,pages:[{page_index:2,width:1200,height:900}]}]}}});await flush();
const pageInput=elements(cards).find(n=>n.tag==='input'&&n.type==='range');assert.ok(pageInput,'native progress slider replaces numeric input');
assert.ok(!elements(cards).some(n=>n.tag==='input'&&n.type==='number'));
const previous=button('Previous image'),next=button('Next image');assert.ok(previous&&next);assert.equal(previous.disabled,true);assert.equal(next.disabled,false);
assert.ok(elements(cards).some(n=>n.className==='page-badge'&&n.textContent==='3'));
const firstPreview=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').at(-1).message;
const firstPreviewImage=elements(cards).find(n=>n.tag==='img');
const beforeInvalid=sent.length;pageInput.value='4';pageInput.events.change();assert.equal(sent.length,beforeInvalid,'out-of-range pages must not call tools');
pageInput.value='3';pageInput.events.input();assert.equal(sent.length,beforeInvalid,'drag previews target without loading');
assert.equal(elements(cards).find(n=>n.className==='page-position').textContent,'3 / 3');
pageInput.events.change();const pagePreview=latest('tools/call');assert.equal(previous.disabled,false);assert.equal(next.disabled,true);
receive({id:firstPreview.id,result});await flush();assert.equal(firstPreviewImage.src,'','superseded image response must not assign a URL');
assert.deepEqual(pagePreview.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42],page:3,quality:'regular'}});
receive({id:pagePreview.id,result:{structuredContent:{media_origin:'https://media.test',images:[{illust_id:42,page:3,url:'https://media.test/media/page-three'}]}}});await flush();
assert.ok(elements(cards).some(n=>n.tag==='img'&&n.src==='https://media.test/media/page-three'));
const thirdImage=elements(cards).find(n=>n.tag==='img'&&n.src==='https://media.test/media/page-three');thirdImage.naturalWidth=1200;thirdImage.naturalHeight=900;thirdImage.events.load();
assert.ok(elements(cards).some(n=>n.className==='metadata-value'&&n.textContent==='1200 × 900'));
const beforeBoundary=sent.length;next.events.click();assert.equal(sent.length,beforeBoundary,'last image must not wrap');
const surface=elements(cards).find(n=>n.className==='page-media');
let prevented=false;
const swipe=(dx,dy)=>{surface.events.pointerdown({pointerId:1,isPrimary:true,button:0,clientX:100,clientY:100});surface.events.pointerup({pointerId:1,clientX:100+dx,clientY:100+dy,preventDefault(){prevented=true;}});};
let captures=0;surface.setPointerCapture=()=>captures++;
surface.events.pointerdown({pointerId:1,isPrimary:true,button:0,clientX:100,clientY:100,target:{closest:()=>({tag:'button'})}});
assert.equal(captures,0,'swipe capture must not intercept retry button clicks');
swipe(-5,100);assert.equal(sent.length,beforeBoundary);assert.equal(prevented,false,'vertical gestures remain available to chat');
swipe(100,5);assert.equal(latest('tools/call').params.arguments.page,2);assert.equal(prevented,true);
previous.events.click();assert.equal(latest('tools/call').params.arguments.page,1);assert.equal(previous.disabled,true);
const beforeFirst=sent.length;previous.events.click();assert.equal(sent.length,beforeFirst,'first image must not wrap');
const beforeReturn=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length;
pageInput.value='3';pageInput.events.change();
assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length,beforeReturn,'returning to a loaded page reuses the image without another media query');
assert.ok(elements(cards).some(n=>n.tag==='img'&&n.src==='https://media.test/media/page-three'&&!n.hidden));

if(downloads){button('Download original page').events.click();const pageDownload=latest('tools/call');assert.deepEqual(pageDownload.params,{name:'pixiv_artwork_media',arguments:{illust_id:42,pages:[3],quality:'original'}});receive({id:pageDownload.id,result:{isError:true}});await flush();}
const beforeReentry=sent.filter(x=>['pixiv_show_gallery','pixiv_illust_detail'].includes(x.message.params?.name)).length;
button('Back to results').events.click();button(record.title).events.click();await flush();
assert.equal(sent.filter(x=>['pixiv_show_gallery','pixiv_illust_detail'].includes(x.message.params?.name)).length,beforeReentry,'reentering detail reuses metadata and selected-page image');
assert.ok(elements(cards).some(n=>n.tag==='img'&&n.src==='https://media.test/media/page-three'&&!n.hidden));
assert.equal(elements(cards).find(n=>n.className==='page-position').textContent,'3 / 3');
const reenteredImage=elements(cards).find(n=>n.tag==='img'&&n.src==='https://media.test/media/page-three');
assert.equal(typeof reenteredImage.events.dblclick,'function','double click opens original viewer');
const reenteredSurface=elements(cards).find(n=>n.className==='page-media');
reenteredSurface.events.dblclick({target:{closest:()=>undefined}});await flush();
const originalCall=latest('tools/call');assert.deepEqual(originalCall.params,{name:'pixiv_artwork_media',arguments:{illust_id:42,pages:[3],quality:'original'}});
receive({id:originalCall.id,result:{content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}],structuredContent:{pages:[{page:3,content_index:0,size:8,mime_type:'image/png'}]}}});await flush();
const originalDialog=elements(cards).find(n=>n.tag==='dialog');assert.ok(originalDialog?.open,'original opens in accessible dialog');
const originalImage=elements(originalDialog).find(n=>n.tag==='img');assert.ok(originalImage.src.startsWith('blob:'));
originalImage.naturalWidth=1200;originalImage.naturalHeight=900;originalImage.events.load();
button('Actual size').events.click();assert.equal(originalImage.className,'original-image actual-size');
if(downloads){const originalReads=sent.filter(x=>x.message.params?.name==='pixiv_artwork_media').length;button('Download viewed original').events.click();await flush();assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_artwork_media').length,originalReads,'viewed original download reuses bytes');receive({id:latest('ui/download-file').id,result:{}});await flush();}
button('Close original').events.click();assert.equal(originalDialog.open,false);
button('View original').events.click();await flush();const hugeOriginal=elements(cards).find(n=>n.tag==='dialog');const hugeImage=elements(hugeOriginal).find(n=>n.tag==='img');hugeImage.naturalWidth=10000;hugeImage.naturalHeight=10000;hugeImage.events.load();
if(downloads){const beforeHugeDownload=sent.filter(x=>x.message.params?.name==='pixiv_artwork_media').length;button('Download viewed original').events.click();await flush();assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_artwork_media').length,beforeHugeDownload,'active oversized original downloads without a second read');receive({id:latest('ui/download-file').id,result:{}});await flush();}
button('Close original').events.click();
const beforeHugeOriginal=sent.filter(x=>x.message.params?.name==='pixiv_artwork_media').length;
button('View original').events.click();await flush();assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_artwork_media').length,beforeHugeOriginal+1,'actual decoded dimensions exceeding budget invalidate retained original bytes');
button('Close original').events.click();

// The approved browser budget evicts least-used images, while an oversized image still displays fully.
receive({method:'ui/notifications/tool-result',params:result});button(record.title).events.click();
receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,page_count:3}]}}});await flush();
async function finishPage(page,width,height){const call=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').at(-1).message;receive({id:call.id,result:{structuredContent:{media_origin:'https://media.test',images:[{illust_id:42,page,url:'https://media.test/media/budget-'+page}]}}});await flush();const image=elements(cards).find(n=>n.tag==='img');image.naturalWidth=width;image.naturalHeight=height;image.events.load();return image;}
const evicted=await finishPage(1,6000,4000);button('Next image').events.click();await finishPage(2,6000,4000);
assert.equal(evicted.src,'','LRU eviction releases detached image');
button('Previous image').events.click();assert.equal(latest('tools/call').params.arguments.page,1,'evicted image is loaded again');
const oversized=await finishPage(1,10000,10000);assert.equal(oversized.hidden,false,'oversized images render without truncation');
button('Next image').events.click();const callsBeforeOversize=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length;
button('Previous image').events.click();assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length,callsBeforeOversize+1,'oversized image is not retained');
for (const [x,label] of [[0,undefined],[2,'R-18G'],[undefined,undefined]]) {
  receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{id:'42',type:'illust',title:'Test',page_count:2,x_restrict:x,tags:[{name:'R-18'}]}]}}});
  assert.equal(elements(cards).find(n=>n.className==='rating-badge')?.textContent,label,'rating comes only from x_restrict');
  assert.ok(elements(cards).some(n=>n.className==='page-badge'&&n.getAttribute('aria-label')==='2 images'));
  const image=elements(cards).find(n=>n.tag==='img');image.events.error();
  assert.equal(elements(cards).find(n=>n.className==='rating-badge')?.textContent,label,'rating survives image failure');
}
receive({method:'ui/notifications/tool-result',params:result});button(record.title).events.click();
const sparseDetail=latest('tools/call');receive({id:sparseDetail.id,result:{structuredContent:{records:[{id:'42',type:'illust',title:'Sparse',page_count:2,published_at:'0001-01-01T00:00:00Z',pages:[{page_index:1,width:600,height:900}]}]}}});await flush();
assert.ok(!elements(cards).some(n=>n.tag==='time'),'Go zero publication time is absent metadata');
assert.ok(!elements(cards).some(n=>n.className==='artwork-author'));
button('Next image').events.click();assert.ok(elements(cards).some(n=>n.className==='metadata-value'&&n.textContent==='600 × 900'),'later page can introduce dimensions absent on page one');
// Stale tool responses cannot update a replaced gallery.
receive({method:'ui/notifications/tool-result',params:result});intersections.at(-1).callback([{isIntersecting:true,target:cards.children[0]}]);await flush();const oldImage=elements(cards).find(n=>n.tag==='img');oldImage.events.error();button('Retry image').events.click();const stale=latest('tools/call');receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[]}}});receive({id:stale.id,result});await flush();assert.equal(cards.children.length,0);
// Gallery animation uses short prepare/status requests and protected binary links.
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{...record,type:'ugoira'}]}}});button(record.title).events.click();const ad=latest('tools/call');receive({id:ad.id,result:{structuredContent:{records:[{...record,type:'ugoira'}]}}});await flush();button('Load animation').events.click();const ac=latest('tools/call');assert.equal(ac.params.name,'pixiv_gallery_animation');assert.deepEqual(ac.params.arguments,{action:'prepare',illust_id:42,format:'gif'});
receive({id:ac.id,result:{structuredContent:{handle:'subscription-1',state:'queued'}}});await flush();
assert.equal(timers.size,1);const scheduled=[...timers.values()][0];assert.equal(scheduled.ms,2000);timers.clear();scheduled.fn();await flush();
const statusCall=latest('tools/call');assert.deepEqual(statusCall.params.arguments,{action:'status',handle:'subscription-1'});
const ready={handle:'subscription-1',state:'ready',media_origin:'https://media.test',animation_url:'https://media.test/media/animation/gif.animation.signature',preview_url:'https://media.test/media/animation/gif.preview.signature',filename:'42.gif',mime_type:'image/gif',size:100};
receive({id:statusCall.id,result:{structuredContent:ready}});await flush();assert.equal(timers.size,0);
const visibleAnimationImage=()=>elements(cards).find(node=>node.tag==='img'&&!node.hidden);
const previewImage=visibleAnimationImage();assert.equal(previewImage.src,ready.preview_url);
const animationMessage=elements(cards).find(node=>node.tag==='p'&&node.textContent==='Loading static preview…');
assert.ok(animationMessage,'ready control response does not imply the preview has loaded');
previewImage.currentSrc=ready.preview_url;previewImage.events.load();assert.equal(animationMessage.textContent,'Static preview.');
button('Play animation').events.click();const animatedImage=visibleAnimationImage();assert.equal(animatedImage.src,ready.animation_url);assert.equal(animationMessage.textContent,'Loading animation…');
previewImage.events.load();assert.equal(animationMessage.textContent,'Loading animation…','a previous preview load cannot announce animation playback');
animatedImage.currentSrc=ready.animation_url;animatedImage.events.load();assert.equal(animationMessage.textContent,'Playing animation.');
button('Show preview').events.click();assert.equal(visibleAnimationImage().src,ready.preview_url);assert.equal(animationMessage.textContent,'Static preview.','loaded preview returns without another loading cycle');
animatedImage.events.load();assert.equal(animationMessage.textContent,'Static preview.','hidden animation load cannot replace the selected preview status');
button('Play animation').events.click();assert.equal(visibleAnimationImage().src,ready.animation_url);assert.equal(animationMessage.textContent,'Playing animation.','loaded animation plays again without another loading cycle');
const beforeImageFailure=sent.length;animatedImage.events.error();assert.equal(animationMessage.textContent,'Animation or preview delivery failed. Retry explicitly.');assert.equal(sent.length,beforeImageFailure,'delivery failure does not silently retry');
button('Show preview').events.click();assert.equal(animationMessage.textContent,'Static preview.');
button('Play animation').events.click();assert.equal(animationMessage.textContent,'Animation or preview delivery failed. Retry explicitly.','switching to failed media does not retry or claim playback');assert.equal(sent.length,beforeImageFailure);
const animationContent={type:'resource_link',uri:ready.animation_url,name:'42.gif',mimeType:'image/gif',size:100};
const downloadButton=button('Download 42.gif');assert.ok(downloadButton);
if(downloads){downloadButton.events.click();const downloadCall=latest('ui/download-file');assert.deepEqual(downloadCall.params,{contents:[animationContent]});receive({id:downloadCall.id,result:{isError:true}});await flush();assert.ok(elements(cards).some(node=>node.textContent.includes('Download not confirmed')));downloadButton.events.click();receive({id:latest('ui/download-file').id,result:{}});await flush();assert.ok(elements(cards).some(node=>node.textContent.includes('Host confirmed download')));}else{assert.equal(downloadButton.disabled,true);}
const animationFormat=elements(cards).find(node=>node.tag==='select');animationFormat.value='apng';animationFormat.events.change();
assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'subscription-1'});
assert.equal(previewImage.src,'','format change releases the retained preview source');assert.equal(animatedImage.src,'','format change releases the retained animation source');
const releasedMessage=animationMessage.textContent;animatedImage.events.load();animatedImage.events.error();assert.equal(animationMessage.textContent,releasedMessage,'released image events cannot change the selected-format prompt');
button('Load animation').events.click();const apngCall=latest('tools/call');assert.deepEqual(apngCall.params.arguments,{action:'prepare',illust_id:42,format:'apng'});
receive({id:apngCall.id,result:{structuredContent:{state:'failed',error:'animation_generation_failed'},isError:true}});await flush();assert.equal(timers.size,0);assert.ok(elements(cards).some(node=>node.textContent.includes('Animation read failed')));
button('Load animation').events.click();const animationRetry=latest('tools/call');receive({id:animationRetry.id,result:{structuredContent:{...ready,handle:'subscription-2',mime_type:'image/apng',filename:'42.apng',animation_url:'https://foreign.test/media/animation/apng.animation.sig'}}});await flush();assert.ok(!elements(cards).some(node=>node.tag==='img'),'foreign media origin is rejected');
button('Load animation').events.click();const final=latest('tools/call');receive({id:final.id,result:{structuredContent:{...ready,handle:'subscription-3',mime_type:'image/apng',filename:'42.apng'}}});await flush();
receive({method:'ui/notifications/tool-result',params:result});assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'subscription-3'});assert.equal(timers.size,0);
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{...record,type:'ugoira'}]}}});button(record.title).events.click();const staleDetail=latest('tools/call');receive({id:staleDetail.id,result:{structuredContent:{records:[{...record,type:'ugoira'}]}}});await flush();button('Load animation').events.click();const stalePrepare=latest('tools/call');receive({method:'ui/notifications/tool-result',params:result});receive({id:stalePrepare.id,result:{structuredContent:{state:'queued',handle:'stale-subscription'}}});await flush();assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'stale-subscription'},'late prepare must release its subscription');


for(const f of listeners.pagehide)f();assert.ok(revoked.every(u=>u.startsWith('blob:')));console.log('Gallery mock: on-demand proxy images, layout, retry, stale responses, animation');
