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
  decode() { if (this.naturalWidth) return Promise.resolve(); return new Promise(resolve => { this.resolveDecode = resolve; }); }
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
  URL: class extends URL { static createObjectURL(blob) { const url = 'blob:fixture-' + created.length; created.push({url, blob}); return url; } static revokeObjectURL(url) { revoked.push(url); } }, Blob, atob,
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
receive({id: initialize.id, result: {protocolVersion:badProtocol?'unsupported':'2026-01-26', hostCapabilities: {...(tools ? {serverTools:{},message:{text:{}}} : {}), ...(downloads ? {downloadFile:{}} : {})}, hostContext:{}}});
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
receive({method:'ui/notifications/tool-input',params:{arguments:{illust_ids:[43]}}});
assert.equal(status.textContent,'Loading results…','new host input resets the old gallery without throwing');
assert.equal(cards.children.length,0);
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

const record = {id:'42',type:'illust',title:'<img src=x onerror=alert(1)>',caption:'First&#44;<br />Second<BR>Third<br/>Fourth\n<b>Description</b> &#x1F600; &#60;img src=x&#62; &#1114112;',page_count:1,x_restrict:1,user:{id:'7',name:'Artist <b>Name</b>'},published_at:'2026-10-01T12:00:00Z',total_views:420,total_bookmarks:0,width:640,height:800,tags:[{name:'tag'}]};
const elements = node => [node, ...node.children.flatMap(elements)];
const settleOriginal = async (dialog, width = 1200, height = 900) => { const pending = elements(dialog).filter(n => n.tag === 'img').find(n => n.resolveDecode); if (pending) { pending.naturalWidth = width; pending.naturalHeight = height; pending.resolveDecode(); pending.resolveDecode = undefined; } await flush(); };
const button = label => elements(cards).find(node => node.tag === 'button' && ((node.getAttribute('aria-label') || node.textContent) === label || node.getAttribute('aria-label') === 'Open ' + label));
const freshResult = value => { receive({method:'ui/notifications/tool-input',params:{arguments:{}}}); receive({method:'ui/notifications/tool-result',params:value}); };
const fullGIF = 'R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';
const mediaPreview = 'iVBORw0KGgo=';
const mediaResult = {structuredContent:{illust_id:42,title:'Animation',complete:true,pages:[{page:1,filename:'42.gif',mime_type:'image/gif',size:Buffer.from(fullGIF,'base64').length,content_index:1,preview_content_index:0}]},content:[{type:'image',mimeType:'image/png',data:mediaPreview},{type:'image',mimeType:'image/gif',data:fullGIF}]};
const callsBeforeMedia = sent.filter(x=>x.message.method==='tools/call').length;
freshResult(mediaResult);
assert.ok(button('Play animation'),'full media result must expose animation playback');
const mediaPreviewImage = elements(cards).find(n=>n.tag==='img'&&!n.hidden);
assert.deepEqual(Buffer.from(await created.find(x=>x.url===mediaPreviewImage.src).blob.arrayBuffer()),Buffer.from(mediaPreview,'base64'));
button('Play animation').click();
const mediaAnimationImage = elements(cards).find(n=>n.tag==='img'&&!n.hidden);
assert.deepEqual(Buffer.from(await created.find(x=>x.url===mediaAnimationImage.src).blob.arrayBuffer()),Buffer.from(fullGIF,'base64'),'play must use the complete indexed GIF, never its preview');
const mediaURLs = created.length, mediaAnimationURL = mediaAnimationImage.src;
button('Show preview').click();button('Play animation').click();
assert.equal(created.length,mediaURLs,'view toggles reuse decoded content sources');
assert.equal(sent.filter(x=>x.message.method==='tools/call').length,callsBeforeMedia,'embedded playback must not query or encode again');
if(downloads){button('Download 42.gif').click();const save=latest('ui/download-file');assert.equal(save.params.contents[0].resource.blob,fullGIF);assert.ok(save.params.contents[0].resource.uri.endsWith('/42.gif'));receive({id:save.id,result:{}});await flush();}
else {assert.equal(button('Download 42.gif'),undefined,'unsupported download shows no control or explanatory text');assert.equal(elements(cards).filter(n=>n.tag==='a').length,0,'missing download capability must not create a fake URL');}
if(!downloads){
 freshResult({...mediaResult,_meta:{'pixiv/mediaDownloads':{media_origin:'https://media.test',pages:[{page:1,url:'https://media.test/media/animation/opaque.animation.signature'}]}}});
 const link=elements(cards).find(n=>n.tag==='a'&&n.getAttribute('aria-label')==='Download 42.gif');
 assert.ok(link,'host without downloadFile must offer the protected original attachment');
 assert.equal(link.href,'https://media.test/media/animation/opaque.animation.signature?download=1');
 freshResult({...mediaResult,_meta:{'pixiv/mediaDownloads':{media_origin:'https://media.test',pages:[{page:1,url:'https://evil.test/file'}]}}});
 assert.equal(elements(cards).filter(n=>n.tag==='a').length,0,'reject cross-origin download metadata');
}
freshResult({structuredContent:{records:[]}});
assert.ok(revoked.includes(mediaAnimationURL),'leaving media must release its Blob URL');
freshResult({...mediaResult,content:[mediaResult.content[0]],_meta:{'pixiv/mediaContents':mediaResult.content}});
assert.ok(button('Play animation'),'App-only inline content must expose full animation playback');
button('Play animation').click();
const privateAnimationImage=elements(cards).find(n=>n.tag==='img'&&!n.hidden);
assert.deepEqual(Buffer.from(await created.find(x=>x.url===privateAnimationImage.src).blob.arrayBuffer()),Buffer.from(fullGIF,'base64'),'App-only delivery preserves every original byte');
if(downloads){button('Download 42.gif').click();const save=latest('ui/download-file');assert.equal(save.params.contents[0].resource.blob,fullGIF);receive({id:save.id,result:{}});await flush();}
freshResult({...mediaResult,_meta:{'pixiv/mediaContents':{...mediaResult.content}}});
assert.ok(elements(cards).some(n=>n.textContent==='Media content is unavailable or invalid.'),'App media must be an array, not an array-like object');
freshResult({...mediaResult,_meta:{'pixiv/mediaContents':{invalid:true}}});
assert.ok(elements(cards).some(n=>n.textContent==='Media content is unavailable or invalid.'),'malformed private content must not fall back silently');
const publishedURL='https://media.test/media/animation/published.animation.signature';
const publishedOriginal={type:'resource_link',uri:publishedURL,name:'42.gif',mimeType:'image/gif',size:Buffer.from(fullGIF,'base64').length};
const publishedResult={...mediaResult,content:[mediaResult.content[0]],_meta:{'pixiv/mediaContents':[mediaResult.content[0],publishedOriginal],'pixiv/mediaDownloads':{media_origin:'https://media.test',pages:[{page:1,url:publishedURL}]}}};
freshResult(publishedResult);assert.ok(button('Play animation'),'published full media must remain playable');
assert.ok(!elements(cards).some(n=>n.tag==='img'&&n.src===publishedURL),'full binary is loaded only after Play');
button('Play animation').click();assert.equal(elements(cards).find(n=>n.tag==='img'&&!n.hidden).src,publishedURL);
if(downloads){button('Download 42.gif').click();const save=latest('ui/download-file');assert.deepEqual(save.params.contents,[publishedOriginal]);receive({id:save.id,result:{}});await flush();}
else assert.equal(elements(cards).find(n=>n.tag==='a').href,publishedURL+'?download=1');
const renewMedia = button('Refresh animation');
assert.ok(renewMedia,'linked media must offer authorization renewal even when only its old download has expired');
assert.equal(renewMedia.disabled,!tools);
if(tools){
 const oldImage=elements(cards).find(n=>n.tag==='img'&&!n.hidden);
 const beforeRenew=sent.filter(x=>x.message.method==='tools/call').length;
 oldImage.events.error();
 assert.equal(sent.filter(x=>x.message.method==='tools/call').length,beforeRenew,'failed media never automatically prepares or retries');
 renewMedia.click();const renewed=latest('tools/call');
 assert.deepEqual(renewed.params,{name:'pixiv_gallery_animation',arguments:{action:'prepare',illust_id:42,format:'gif'}},'explicit refresh must request a current authorized animation instead of repeating an expired URL');
 assert.equal(oldImage.src,'','renewal releases the obsolete source');
 const freshURL='https://media.test/media/animation/renewed.animation.signature';
 receive({id:renewed.id,result:{structuredContent:{handle:'media-renewal',state:'ready',animation_url:freshURL,preview_url:'https://media.test/media/animation/renewed.preview.signature',media_origin:'https://media.test',filename:'42.gif',mime_type:'image/gif',size:100}}});await flush();
 button('Play animation').click();assert.equal(elements(cards).find(n=>n.tag==='img'&&!n.hidden).src,freshURL);
 if(downloads){button('Download 42.gif').click();const save=latest('ui/download-file');assert.equal(save.params.contents[0].uri,freshURL);receive({id:save.id,result:{}});await flush();}
 else assert.equal(elements(cards).find(n=>n.tag==='a').href,freshURL+'?download=1');
 freshResult({structuredContent:{records:[]}});
 assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'media-renewal'},'leaving renewed media releases its subscription');
}
if(tools){
 freshResult({...publishedResult,structuredContent:{...mediaResult.structuredContent,pages:[{...mediaResult.structuredContent.pages[0],filename:'42.apng',mime_type:'image/apng'}]},_meta:{...publishedResult._meta,'pixiv/mediaContents':[mediaResult.content[0],{...publishedOriginal,mimeType:'image/apng',name:'42.apng'}]}});
 button('Refresh animation').click();const preparing=latest('tools/call');
 assert.equal(preparing.params.arguments.format,'apng','refresh preserves the original format even after an earlier GIF view');
 freshResult({structuredContent:{records:[]}});
 receive({id:preparing.id,result:{structuredContent:{handle:'late-renewal',state:'queued'}}});await flush();
 assert.equal(cards.children.length,0,'late renewed result cannot replace a newer view');
 assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'late-renewal'});
}
freshResult({...publishedResult,_meta:{...publishedResult._meta,'pixiv/mediaContents':[mediaResult.content[0],{...publishedOriginal,uri:'https://evil.test/media/animation/file'}]}});
assert.equal(button('Play animation'),undefined,'reject a full-media link outside the authorized download metadata');
freshResult({...publishedResult,_meta:{...publishedResult._meta,'pixiv/mediaContents':[mediaResult.content[0],{...publishedOriginal,size:publishedOriginal.size+1}]}});
assert.equal(button('Play animation'),undefined,'reject mismatched full-media byte metadata');
freshResult({...mediaResult,content:[mediaResult.content[1],mediaResult.content[1]]});
assert.ok(elements(cards).some(n=>n.textContent==='Media content is unavailable or invalid.'),'a GIF must not be presented as the independent static PNG preview');
assert.equal(button('Play animation'),undefined);
freshResult({structuredContent:{records:[]}});
// Two records keep the result-card view; a single artwork opens detail directly when tools are available.
const result = {structuredContent:{records:[record,{...record,id:'43',title:'Second artwork'}],media_origin:'https://media.test',images:[{illust_id:42,page:1,url:'https://media.test/media/signed.fixture'},{illust_id:43,page:1,url:'https://media.test/media/second.fixture'}]},content:[]};
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[record,{...record,id:'43',title:'Second artwork',user:{name:'Second artist'}}],media_origin:'https://media.test',images:[{illust_id:42,page:1,url:'https://media.test/media/first'},{illust_id:43,page:1,url:'https://media.test/media/second'}]}}});
for(const target of cards.children)intersections.at(-1).callback([{isIntersecting:true,target}]);
await flush();
if(tools){const batch=latest('tools/call');assert.deepEqual(batch.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42,43],page:1,quality:'thumbnail'}},'historical thumbnails are renewed in one batch');receive({id:batch.id,result:{structuredContent:{media_origin:'https://media.test',images:[{illust_id:42,page:1,url:'https://media.test/media/first'},{illust_id:43,page:1,url:'https://media.test/media/second'}]}}});await flush();}
assert.deepEqual(elements(cards).filter(n=>n.tag==='img').map(n=>n.src),['https://media.test/media/first','https://media.test/media/second'],'each card receives its own artwork image');
assert.ok(button('Second artwork'));
receive({method:'ui/notifications/tool-input',params:{arguments:{illust_ids:[44]}}});
assert.equal(cards.children.length,0,'new input clears the previous artwork cards');
assert.ok(!html.includes('<h1>Pixiv Gallery</h1>'));
freshResult(result);
assert.equal(status.textContent,'','successful gallery has no result-count heading');
const card=cards.children[0]; assert.equal(card.children[0].className,'media-frame','image comes first');
assert.ok(!elements(card).some(n=>n.tag==='h2'||n.className==='artwork-author'||n.className==='tags collapsed'),'list displays thumbnails without text metadata');
assert.ok(!button('Open artwork'));assert.ok(!button('Load / retry preview'));
const retry=elements(card).find(n=>n.tag==='button' && n.getAttribute('aria-label')==='Retry image');assert.equal(retry.hidden,true);
assert.ok(!elements(card).some(n=>n.textContent===record.caption),'list keeps description in detail');
assert.ok(!elements(card).some(n=>['artwork-author','rating-badge','page-badge','tag-list'].includes(n.className)),'thumbnail-only list has no metadata overlays');
assert.equal(sent.filter(x=>x.message.method==='tools/call'&&x.message.params.name==='pixiv_show_gallery').length,tools?1:0);
intersections.at(-1).callback([{isIntersecting:true,target:card}]);await flush();
if(tools){const batch=latest('tools/call');assert.equal(elements(card).find(n=>n.tag==='img').src,'','do not use historical signed URLs before renewal');receive({id:batch.id,result});await flush();}
const img=elements(card).find(n=>n.tag==='img');assert.equal(img.src,'https://media.test/media/signed.fixture');assert.equal(img.referrerPolicy,'no-referrer');
img.events.load();assert.equal(retry.hidden,true);img.events.error();assert.equal(retry.hidden,false);
if(!tools){assert.equal(retry.disabled,true);for(const f of listeners.pagehide)f();console.log('Gallery mock: images without tool-call capability');process.exit(0)}
retry.events.click();const renewal=latest('tools/call');assert.deepEqual(renewal.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42],page:1,quality:'thumbnail'}});
receive({id:renewal.id,result});await flush();img.events.load();assert.equal(retry.hidden,true);
// One retry renews every failed card of the gallery in a single batch.
const second=cards.children[1];intersections.at(-1).callback([{isIntersecting:true,target:second}]);await flush();
const secondImg=elements(second).find(n=>n.tag==='img');secondImg.events.error();img.events.error();
retry.events.click();const batchRenewal=latest('tools/call');assert.deepEqual(batchRenewal.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42,43],page:1,quality:'thumbnail'}});
receive({id:batchRenewal.id,result});await flush();assert.equal(secondImg.src,'https://media.test/media/second.fixture','other failed cards reload with the same batch');img.events.load();secondImg.events.load();
button(record.title).events.click();const detail=latest('tools/call');assert.equal(detail.params.name,'pixiv_illust_detail');receive({id:detail.id,result:{structuredContent:{records:[record]},content:[]}});await flush();
assert.equal(status.textContent,'','successful detail has no generic heading');
assert.equal(button('View original'),undefined,'original viewing belongs to the image instead of a separate button');
const detailSection=cards.children.find(n=>n.tag==='article');
assert.equal(detailSection.className,'artwork-detail','detail has its own responsive layout');
assert.ok(elements(detailSection).some(n=>n.className==='detail-media'));
assert.ok(elements(detailSection).some(n=>n.className==='detail-info'));
assert.ok(!elements(detailSection).some(n=>n.tag==='details'||n.tag==='summary'),'detail has no disclosure controls');
assert.ok(elements(detailSection).some(n=>n.className==='caption'&&n.textContent==='First,\nSecond\nThird\nFourth\n<b>Description</b> 😀 <img src=x> &#1114112;'),'caption decodes valid numeric entities and line breaks, retaining markup and invalid entities as inert text');
assert.ok(elements(detailSection).some(n=>n.className==='artwork-author'&&n.textContent===record.user.name));
const authorButton=elements(detailSection).find(n=>n.tag==='button'&&n.className==='artwork-author');assert.ok(authorButton,'known author is clickable');authorButton.events.click();assert.deepEqual(latest('ui/open-link').params,{url:'https://www.pixiv.net/users/7'});receive({id:latest('ui/open-link').id,result:{}});await flush();
const metadata=elements(detailSection).filter(n=>n.className==='metadata-value').map(n=>n.textContent);
assert.ok(!elements(detailSection).some(n=>n.tag==='dt'&&n.textContent==='Pages'),'detail omits the redundant Pages metadata field');
for(const value of ['420','0','640 × 800'])assert.ok(metadata.includes(value),'metadata includes '+value);
assert.equal(button('Copy artwork ID').textContent,'42');
assert.ok(elements(detailSection).some(n=>n.tag==='time'&&n.dateTime===record.published_at));
assert.ok(elements(detailSection).some(n=>n.className==='rating-badge'&&n.textContent==='R-18'));
const tagButton=elements(detailSection).find(n=>n.tag==='button'&&n.className==='tag');
assert.ok(tagButton,'message-capable host exposes real tag actions');tagButton.click();
const tagMessage=latest('ui/message');assert.equal(tagMessage.params.role,'user');
assert.ok(tagMessage.params.content[0].text.includes(JSON.stringify('tag')));
receive({id:tagMessage.id,result:{isError:true}});await flush();
assert.ok(status.textContent.includes('Tag search could not be sent'));assert.equal(tagButton.disabled,false);
const back=button('Back to results');assert.ok(back.getAttribute('aria-label'));assert.equal(back.title,'Back to results');assert.ok(elements(back).some(n=>n.tag==='svg'&&n.getAttribute('aria-hidden')==='true'));
const bookmark=button('Bookmark'),remove=button('Remove bookmark'),refresh=button('Refresh bookmark');
const visibility=elements(cards).find(n=>n.tag==='select'&&n.getAttribute('aria-label')==='Bookmark visibility');
assert.ok(visibility);assert.equal(bookmark.disabled,true,'unknown state prohibits bookmark writes');assert.equal(remove.hidden,true);
const query=latest('tools/call');assert.equal(query.params.name,'pixiv_bookmark_detail');
receive({id:query.id,result:{structuredContent:{bookmarked:false,restrict:'public',tags:['keep']}}});await flush();
assert.equal(bookmark.disabled,false);assert.equal(remove.hidden,true);
const options=elements(cards).find(n=>n.className==='bookmark-options');
assert.ok(options?.hidden,'advanced bookmark options are hidden until long press');
const beforeHold=sent.filter(x=>x.message.method==='tools/call').length;
bookmark.events.pointerdown({button:0,isPrimary:true,pointerId:1,clientX:0,clientY:0});
const hold=[...timers.entries()].find(([,timer])=>timer.ms===500);assert.ok(hold);timers.delete(hold[0]);hold[1].fn();
assert.equal(options.hidden,false);bookmark.events.pointerup();bookmark.events.click();
assert.equal(sent.filter(x=>x.message.method==='tools/call').length,beforeHold,'long press and its following click must not write');
visibility.value='private';button('Apply bookmark settings').click();
const add=latest('tools/call');assert.deepEqual(add.params,{name:'pixiv_bookmark_add',arguments:{kind:'artwork',id:42,restrict:'private',tags:['keep']}});
assert.equal(bookmark.disabled,true);receive({id:add.id,result:{}});await flush();
const confirm=latest('tools/call');assert.equal(confirm.params.name,'pixiv_bookmark_detail');
receive({id:confirm.id,result:{structuredContent:{bookmarked:true,restrict:'private',tags:['keep']}}});await flush();
receive({id:latest('tools/call').id,result:{structuredContent:{records:[record]}}});await flush();
assert.ok(button('Unbookmark'));assert.equal(remove.hidden,false);assert.equal(visibility.value,'private');
remove.events.click();const removal=latest('tools/call');assert.deepEqual(removal.params,{name:'pixiv_bookmark_remove',arguments:{kind:'artwork',id:42}});
receive({id:removal.id,result:{isError:true}});await flush();assert.equal(bookmark.disabled,true,'unconfirmed write requires state refresh');
assert.ok(elements(cards).some(n=>n.textContent.includes('Bookmark change not confirmed')));assert.equal(bookmark.hidden,true);assert.equal(button('Retry bookmark state').hidden,false,'unknown state shows only an icon retry');
refresh.events.click();const recovery=latest('tools/call');receive({id:recovery.id,result:{structuredContent:{bookmarked:false,restrict:'public',tags:[]}}});await flush();assert.equal(bookmark.disabled,false);
assert.equal(button('Load page'),undefined,'single page has no page controls');assert.ok(!elements(cards).some(n=>n.tag==='input'&&n.type==='number'));
const detailImageCall=sent.filter(x=>x.message.method==='tools/call'&&x.message.params.name==='pixiv_show_gallery').at(-1).message;
receive({id:detailImageCall.id,result});await flush();assert.ok(elements(cards).some(n=>n.tag==='img'&&n.src==='https://media.test/media/signed.fixture'));
if(downloads){button('Download original page').events.click();const bytesCall=latest('tools/call');receive({id:bytesCall.id,result:{content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}],structuredContent:{pages:[{page:1,content_index:0}]}}});await flush();const save=latest('ui/download-file');receive({id:save.id,result:{isError:true}});await flush();assert.ok(elements(cards).some(n=>n.textContent==='Download failed. Retry explicitly.'));
button('Download original page').events.click();const secondBytes=latest('tools/call');receive({id:secondBytes.id,result:{content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}],structuredContent:{pages:[{page:1,content_index:0}]}}});await flush();
const acceptedDownload=latest('ui/download-file');receive({id:acceptedDownload.id,result:{}});await flush();
assert.ok(elements(cards).some(n=>n.textContent==='Download requested.'),'confirmed original download must not report failure');}
// Multipage detail keeps the selected page for preview and original download.
freshResult(result);button(record.title).events.click();
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
assert.ok(cards.contains(thirdImage) && !thirdImage.hidden && thirdImage.src==='https://media.test/media/page-three','keep the displayed page until its replacement has loaded');
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
receive({method:'ui/notifications/tool-result',params:result});
assert.equal(elements(cards).find(n=>n.className==='page-position')?.textContent,'3 / 3','replayed host result must preserve the current detail and cached page');
const reenteredImage=elements(cards).find(n=>n.tag==='img'&&n.src==='https://media.test/media/page-three');
assert.equal(typeof reenteredImage.events.dblclick,'function','double click opens original viewer');
const reenteredSurface=elements(cards).find(n=>n.className==='page-media');
reenteredSurface.events.dblclick({target:{closest:()=>undefined}});await flush();
const originalCall=latest('tools/call');assert.deepEqual(originalCall.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42],page:3,quality:'original'}},'original preview must stream over HTTP instead of waiting for Base64 tool content');
const originalResult=page=>({structuredContent:{media_origin:'https://media.test',images:[{illust_id:42,page,url:'https://media.test/pixiv/media/original-'+page}]}});
receive({id:originalCall.id,result:originalResult(3)});await flush();
const originalDialog=elements(cards).find(n=>n.tag==='dialog');assert.ok(originalDialog?.open,'original opens in accessible dialog');
await settleOriginal(originalDialog);
const originalImage=elements(originalDialog).find(n=>n.tag==='img'&&!n.hidden);
assert.equal(originalImage.src,'https://media.test/pixiv/media/original-3');
assert.equal(elements(originalDialog).find(n=>n.className==='original-loading').hidden,true);
assert.equal(elements(originalDialog).find(n=>n.className==='original-count').children.at(-1).textContent,'3 / 3');
const lightboxButton=label=>elements(originalDialog).find(n=>n.tag==='button'&&n.getAttribute('aria-label')===label);
assert.equal(lightboxButton('Previous image').disabled,false);assert.equal(lightboxButton('Next image').disabled,true);
if(!downloads){
  const browserSave=elements(originalDialog).find(n=>n.tag==='a'&&n.getAttribute('aria-label')==='Download viewed original');
  assert.ok(browserSave&&!browserSave.hidden);
  assert.equal(browserSave.href,originalImage.src+'?download=1','browser saving reuses the validated original capability');
  assert.equal(browserSave.target,'_blank');assert.equal(browserSave.rel,'noreferrer');
}
lightboxButton('Actual size').events.click();assert.equal(originalImage.className,'original-image original-actual');
assert.equal(elements(originalDialog).find(n=>n.className==='original-zoom').textContent,'1:1');
lightboxButton('Fit to window').events.click();assert.equal(originalImage.className,'original-image');
lightboxButton('Zoom in').events.click();assert.equal(elements(originalDialog).find(n=>n.className==='original-zoom').textContent,'125%');
for(let i=0;i<20;i++)originalDialog.events.keydown({key:'+',preventDefault(){}});
assert.equal(elements(originalDialog).find(n=>n.className==='original-zoom').textContent,'400%','manual zoom is capped at 400%');
originalDialog.events.keydown({key:'0',preventDefault(){}});assert.equal(elements(originalDialog).find(n=>n.className==='original-zoom').textContent,'Fit');
if(downloads){
  lightboxButton('Download viewed original').events.click();await flush();
  const read=latest('tools/call');assert.deepEqual(read.params,{name:'pixiv_artwork_media',arguments:{illust_id:42,pages:[3],quality:'original'}},'host download retrieves bytes only on explicit request');
  receive({id:read.id,result:{content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}],structuredContent:{pages:[{page:3,content_index:0,size:8,mime_type:'image/png'}]}}});await flush();
  assert.equal(latest('ui/download-file').params.contents[0].resource.blob,'iVBORw0KGgo=');
  receive({id:latest('ui/download-file').id,result:{}});await flush();
}
lightboxButton('Previous image').events.click();const previousCall=latest('tools/call');assert.deepEqual(previousCall.params,{name:'pixiv_show_gallery',arguments:{illust_ids:[42],page:2,quality:'original'}});
assert.equal(originalImage.src,'https://media.test/pixiv/media/original-3','old page stays visible while loading');
assert.equal(elements(originalDialog).find(n=>n.className==='original-loading').hidden,false);
assert.equal(originalDialog['aria-busy'],'true');
receive({id:previousCall.id,result:originalResult(2)});await flush();await settleOriginal(originalDialog,800,600);
assert.equal(originalDialog['aria-busy'],'false');
const forwardBefore=sent.length;
originalDialog.events.keydown({key:'ArrowRight',preventDefault(){}});await flush();
assert.equal(sent.length,forwardBefore,'return to a decoded original reuses its image without another tool call');
assert.ok(elements(originalDialog).includes(originalImage),'decoded image element is retained');
lightboxButton('Close original').events.click();assert.equal(originalDialog.open,false);
// A new result clears retained capabilities; reject untrusted and late original responses.
freshResult(result);button(record.title).events.click();receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,page_count:3}]}}});await flush();
elements(cards).find(n=>n.tag==='img'&&n.getAttribute('role')==='button').events.click();await flush();
const invalidDialog=elements(cards).find(n=>n.tag==='dialog');
receive({id:latest('tools/call').id,result:{structuredContent:{media_origin:'https://media.test',images:[{illust_id:42,page:1,url:'https://untrusted.test/media/original'}]}}});await flush();
assert.ok(elements(invalidDialog).filter(n=>n.tag==='img').every(n=>!n.src),'untrusted original URLs never reach an image');
button('Retry image').events.click();const lateCapability=latest('tools/call');button('Close original').events.click();
receive({id:lateCapability.id,result:originalResult(1)});await flush();assert.ok(!cards.contains(invalidDialog),'late responses cannot reopen a closed viewer');
elements(cards).find(n=>n.tag==='img'&&n.getAttribute('role')==='button').events.click();await flush();
receive({id:latest('tools/call').id,result:originalResult(1)});await flush();
const hugeOriginal=elements(cards).find(n=>n.tag==='dialog');await settleOriginal(hugeOriginal,10000,10000);button('Close original').events.click();
const beforeHugeOriginal=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length;
elements(cards).find(n=>n.tag==='img'&&n.getAttribute('role')==='button').events.click();await flush();assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length,beforeHugeOriginal+1,'oversized decoded originals display fully but are not retained');
button('Close original').events.click();
// The approved browser budget evicts least-used images, while an oversized image still displays fully.
freshResult(result);button(record.title).events.click();
receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,page_count:3}]}}});await flush();
async function finishPage(page,width,height){const call=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').at(-1).message;receive({id:call.id,result:{structuredContent:{media_origin:'https://media.test',images:[{illust_id:42,page,url:'https://media.test/media/budget-'+page}]}}});await flush();const image=elements(cards).find(n=>n.tag==='img'&&n.src==='https://media.test/media/budget-'+page);image.naturalWidth=width;image.naturalHeight=height;image.events.load();return image;}
const evicted=await finishPage(1,6000,4000);button('Next image').events.click();await finishPage(2,6000,4000);
assert.equal(evicted.src,'','LRU eviction releases detached image');
button('Previous image').events.click();assert.equal(latest('tools/call').params.arguments.page,1,'evicted image is loaded again');
const oversized=await finishPage(1,10000,10000);assert.equal(oversized.hidden,false,'oversized images render without truncation');
button('Next image').events.click();const callsBeforeOversize=sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length;
button('Previous image').events.click();assert.equal(sent.filter(x=>x.message.params?.name==='pixiv_show_gallery').length,callsBeforeOversize+1,'oversized image is not retained');
for (const [x,label] of [[0,undefined],[2,'R-18G'],[undefined,undefined]]) {
  receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{id:'42',type:'illust',title:'Test',page_count:2,x_restrict:x,tags:[{name:'R-18'}]},{id:'43',type:'illust',title:'Other'}]}}});
  assert.ok(!elements(cards).some(n=>n.className==='rating-badge'),'list has only thumbnails');
  button('Test').events.click();receive({id:latest('tools/call').id,result:{structuredContent:{records:[{id:'42',type:'illust',title:'Test',page_count:2,x_restrict:x,tags:[{name:'R-18'}]}]}}});await flush();
  assert.equal(elements(cards).find(n=>n.className==='rating-badge')?.textContent,label,'rating comes only from x_restrict');
  assert.ok(elements(cards).some(n=>n.className==='page-badge'&&n.getAttribute('aria-label')==='2 images'));
  const image=elements(cards).find(n=>n.tag==='img');image.events.error();
  assert.equal(elements(cards).find(n=>n.className==='rating-badge')?.textContent,label,'rating survives image failure');
}
freshResult(result);button(record.title).events.click();
const sparseDetail=latest('tools/call');receive({id:sparseDetail.id,result:{structuredContent:{records:[{id:'42',type:'illust',title:'Sparse',page_count:2,published_at:'0001-01-01T00:00:00Z',pages:[{page_index:1,width:600,height:900}]}]}}});await flush();
assert.ok(!elements(cards).some(n=>n.tag==='time'),'Go zero publication time is absent metadata');
assert.ok(!elements(cards).some(n=>n.className==='artwork-author'));
button('Next image').events.click();assert.ok(elements(cards).some(n=>n.className==='metadata-value'&&n.textContent==='600 × 900'),'later page can introduce dimensions absent on page one');
// Stale tool responses cannot update a replaced gallery.
freshResult(result);intersections.at(-1).callback([{isIntersecting:true,target:cards.children[0]}]);await flush();const oldImage=elements(cards).find(n=>n.tag==='img');oldImage.events.error();button('Retry image').events.click();const stale=latest('tools/call');receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[]}}});receive({id:stale.id,result});await flush();assert.equal(cards.children.length,0);
// Gallery animation uses short prepare/status requests and protected binary links.
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{...record,type:'ugoira'},{...record,id:'43',title:'Second artwork'}]}}});assert.ok(elements(cards).some(n=>n.tag==='img'),'animation list displays a thumbnail');button(record.title).events.click();const ad=latest('tools/call');receive({id:ad.id,result:{structuredContent:{records:[{...record,type:'ugoira'}]}}});await flush();button('Load animation').events.click();const ac=latest('tools/call');assert.equal(ac.params.name,'pixiv_gallery_animation');assert.deepEqual(ac.params.arguments,{action:'prepare',illust_id:42,format:'gif'});
receive({id:ac.id,result:{structuredContent:{handle:'subscription-1',state:'queued'}}});await flush();
assert.equal(timers.size,1);const scheduled=[...timers.values()][0];assert.equal(scheduled.ms,2000);timers.clear();scheduled.fn();await flush();
const statusCall=latest('tools/call');assert.deepEqual(statusCall.params.arguments,{action:'status',handle:'subscription-1'});
const ready={handle:'subscription-1',state:'ready',media_origin:'https://media.test',animation_url:'https://media.test/media/animation/gif.animation.signature',preview_url:'https://media.test/media/animation/gif.preview.signature',filename:'42.gif',mime_type:'image/gif',size:100};
receive({id:statusCall.id,result:{structuredContent:ready}});await flush();assert.equal(timers.size,0);
assert.equal(button('Load animation').hidden,true,'ready animation hides the redundant prepare action');
const visibleAnimationImage=()=>elements(cards).find(node=>node.tag==='img'&&!node.hidden);
const previewImage=visibleAnimationImage();assert.equal(previewImage.src,ready.preview_url);
const animationMessage=elements(cards).find(node=>node.tag==='p'&&node.textContent==='Loading static preview…');
assert.ok(animationMessage,'ready control response does not imply the preview has loaded');
previewImage.currentSrc=ready.preview_url;previewImage.events.load();assert.equal(animationMessage.textContent,'Static preview.');
button('Play animation').events.click();const animatedImage=visibleAnimationImage();assert.equal(animatedImage.src,ready.animation_url);assert.equal(animationMessage.textContent,'Loading animation…');
previewImage.events.load();assert.equal(animationMessage.textContent,'Loading animation…','a previous preview load cannot announce animation playback');
animatedImage.currentSrc=ready.animation_url;animatedImage.events.load();assert.equal(animationMessage.textContent,'Playing animation.');
if(downloads){const callsBeforeFailure=sent.filter(x=>x.message.method==='tools/call').length;button('Download 42.gif').events.click();receive({id:latest('ui/download-file').id,result:{isError:true}});await flush();assert.equal(button('Load animation').hidden,false,'unconfirmed host download restores explicit preparation without automatic retry');assert.equal(animationMessage.textContent,'Playing animation.','download failure does not discard already loaded media');assert.equal(sent.filter(x=>x.message.method==='tools/call').length,callsBeforeFailure,'failed download does not automatically prepare or retry');}
button('Show preview').events.click();assert.equal(visibleAnimationImage().src,ready.preview_url);assert.equal(animationMessage.textContent,'Static preview.','loaded preview returns without another loading cycle');
animatedImage.events.load();assert.equal(animationMessage.textContent,'Static preview.','hidden animation load cannot replace the selected preview status');
button('Play animation').events.click();assert.equal(visibleAnimationImage().src,ready.animation_url);assert.equal(animationMessage.textContent,'Playing animation.','loaded animation plays again without another loading cycle');
const beforeImageFailure=sent.length;animatedImage.events.error();assert.equal(button('Load animation').hidden,false,'media delivery failure restores explicit retry');assert.equal(animationMessage.textContent,'Animation or preview delivery failed. Retry explicitly.');assert.equal(sent.length,beforeImageFailure,'delivery failure does not silently retry');
button('Show preview').events.click();assert.equal(animationMessage.textContent,'Static preview.');
button('Play animation').events.click();assert.equal(animationMessage.textContent,'Animation or preview delivery failed. Retry explicitly.','switching to failed media does not retry or claim playback');assert.equal(sent.length,beforeImageFailure);
const animationContent={type:'resource_link',uri:ready.animation_url,name:'42.gif',mimeType:'image/gif',size:100};
const downloadButton=downloads?button('Download 42.gif'):undefined;
if(downloads){downloadButton.events.click();const downloadCall=latest('ui/download-file');assert.deepEqual(downloadCall.params,{contents:[animationContent]});receive({id:downloadCall.id,result:{isError:true}});await flush();assert.ok(elements(cards).some(node=>node.textContent.includes('Download not confirmed')));downloadButton.events.click();receive({id:latest('ui/download-file').id,result:{}});await flush();assert.ok(elements(cards).some(node=>node.textContent.includes('Host confirmed download')));}else{const save=elements(cards).find(n=>n.tag==='a'&&n.getAttribute('aria-label')==='Download 42.gif');assert.ok(save,'animation offers an explicit browser download without host downloadFile');assert.equal(save.href,ready.animation_url+'?download=1');assert.equal(save.rel,'noreferrer');assert.ok(!elements(cards).some(n=>n.textContent.includes('media bytes remain available in chat')));}
let obsoleteDownload;if(downloads){downloadButton.events.click();obsoleteDownload=latest('ui/download-file');}
const releasedLoad=animatedImage.events.load,releasedError=animatedImage.events.error;
const animationFormat=elements(cards).find(node=>node.tag==='select');animationFormat.value='apng';animationFormat.events.change();
assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'subscription-1'});
assert.equal(button('Load animation').hidden,false,'format change restores preparation for the selected format');
assert.equal(previewImage.src,ready.preview_url,'format change retains successful preview in the session cache');assert.equal(animatedImage.src,'','format change releases the retained animation source');
const releasedMessage=animationMessage.textContent;releasedLoad();releasedError();assert.equal(animationMessage.textContent,releasedMessage,'released image events cannot change the selected-format prompt');
button('Load animation').events.click();const apngCall=latest('tools/call');assert.deepEqual(apngCall.params.arguments,{action:'prepare',illust_id:42,format:'apng'});
receive({id:apngCall.id,result:{structuredContent:{state:'failed',error:'animation_generation_failed'},isError:true}});await flush();assert.equal(timers.size,0);assert.ok(elements(cards).some(node=>node.textContent.includes('Animation read failed')));assert.equal(button('Load animation').hidden,false,'preparation failure keeps explicit retry available');
button('Load animation').events.click();const animationRetry=latest('tools/call');receive({id:animationRetry.id,result:{structuredContent:{...ready,handle:'subscription-2',mime_type:'image/apng',filename:'42.apng',animation_url:'https://foreign.test/media/animation/apng.animation.sig'}}});await flush();assert.ok(!elements(cards).some(node=>node.tag==='img'),'foreign media origin is rejected');
button('Load animation').events.click();const final=latest('tools/call');receive({id:final.id,result:{structuredContent:{...ready,handle:'subscription-3',mime_type:'image/apng',filename:'42.apng'}}});await flush();
if(obsoleteDownload){receive({id:obsoleteDownload.id,result:{isError:true}});await flush();assert.equal(button('Load animation').hidden,true,'old-format download failure cannot change the new ready view');}
freshResult(result);assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'subscription-3'});assert.equal(timers.size,0);
receive({method:'ui/notifications/tool-result',params:{structuredContent:{records:[{...record,type:'ugoira'}]}}});const staleDetail=latest('tools/call');receive({id:staleDetail.id,result:{structuredContent:{records:[{...record,type:'ugoira'}]}}});await flush();button('Load animation').events.click();const stalePrepare=latest('tools/call');freshResult(result);receive({id:stalePrepare.id,result:{structuredContent:{state:'queued',handle:'stale-subscription'}}});await flush();assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'stale-subscription'},'late prepare must release its subscription');


// Reentering an animation reauthorizes before reusing successful loaded media.
freshResult({structuredContent:{records:[{...record,type:'ugoira'},{...record,id:'43',title:'Second artwork'}]}});
button(record.title).events.click();receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,type:'ugoira'}]}}});await flush();
button('Load animation').events.click();receive({id:latest('tools/call').id,result:{structuredContent:{...ready,handle:'reentry-first'},_meta:{'pixiv/animationFrameCount':2}}});await flush();
const retainedPreview=elements(cards).find(n=>n.tag==='img'&&!n.hidden);retainedPreview.events.load();
button('Play animation').events.click();const retainedAnimation=elements(cards).find(n=>n.tag==='img'&&!n.hidden);retainedAnimation.events.load();
button('Back to results').events.click();assert.deepEqual(latest('tools/call').params.arguments,{action:'release',handle:'reentry-first'});
assert.equal(retainedAnimation.src,ready.animation_url,'leaving detail retains successful animation within the session budget');
button(record.title).events.click();await flush();
const reentryPrepare=sent.filter(x=>x.message.method==='tools/call'&&x.message.params.name==='pixiv_gallery_animation'&&x.message.params.arguments.action==='prepare').at(-1).message;assert.deepEqual(reentryPrepare.params.arguments,{action:'prepare',illust_id:42,format:'gif'},'reentry checks current authorization before reusing media');
assert.ok(!elements(cards).includes(retainedAnimation),'cached animation is not shown before reauthorization');
receive({id:reentryPrepare.id,result:{structuredContent:{...ready,handle:'reentry-second'},_meta:{'pixiv/animationFrameCount':2}}});await flush();
assert.equal(elements(cards).find(n=>n.tag==='img'&&!n.hidden),retainedPreview,'ready reentry reuses the loaded preview element');
assert.ok(elements(cards).some(n=>n.textContent==='Static preview.'),'cached preview has no new loading cycle');
button('Play animation').events.click();assert.equal(elements(cards).find(n=>n.tag==='img'&&!n.hidden),retainedAnimation);
assert.ok(elements(cards).some(n=>n.textContent==='Playing animation.'),'cached animation has no new loading cycle');
button('Back to results').events.click();button(record.title).events.click();await flush();
const changedReentry=sent.filter(x=>x.message.method==='tools/call'&&x.message.params.name==='pixiv_gallery_animation'&&x.message.params.arguments.action==='prepare').at(-1).message;
const changedArtifact={...ready,handle:'reentry-changed',animation_url:ready.animation_url+'.new',preview_url:ready.preview_url+'.new'};
receive({id:changedReentry.id,result:{structuredContent:changedArtifact,_meta:{'pixiv/animationFrameCount':2}}});await flush();
const changedPreview=elements(cards).find(n=>n.tag==='img'&&!n.hidden);assert.notEqual(changedPreview,retainedPreview,'a newly signed artifact cannot reuse an old preview');assert.equal(changedPreview.src,changedArtifact.preview_url);changedPreview.events.load();
button('Play animation').events.click();const changedAnimation=elements(cards).find(n=>n.tag==='img'&&!n.hidden);assert.notEqual(changedAnimation,retainedAnimation);assert.equal(changedAnimation.src,changedArtifact.animation_url);changedAnimation.events.load();
button('Back to results').events.click();button(record.title).events.click();await flush();
const deniedReentry=sent.filter(x=>x.message.method==='tools/call'&&x.message.params.name==='pixiv_gallery_animation'&&x.message.params.arguments.action==='prepare').at(-1).message;
receive({id:deniedReentry.id,result:{isError:true,structuredContent:{state:'expired'}}});await flush();
assert.ok(!elements(cards).some(n=>n.tag==='img'),'failed reauthorization never displays retained animation');
assert.equal(button('Load animation').hidden,false,'failed reentry remains explicitly retryable');
freshResult(result);assert.equal(retainedAnimation.src,'','new query releases retained animation media');assert.equal(retainedPreview.src,'');assert.equal(changedAnimation.src,'');assert.equal(changedPreview.src,'');

// Compressed bytes, decoded frame pixels and missing frame hints all constrain retention.
for(const [size,frames] of [[100,1000],[257*1024*1024,1],[100,undefined]]){
  freshResult({structuredContent:{records:[{...record,type:'ugoira'},{...record,id:'43',title:'Second artwork'}]}});
  button(record.title).events.click();receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,type:'ugoira'}]}}});await flush();
  button('Load animation').events.click();receive({id:latest('tools/call').id,result:{structuredContent:{...ready,handle:'not-retained',size},_meta:{'pixiv/animationFrameCount':frames}}});await flush();
  const smallPreview=elements(cards).find(n=>n.tag==='img'&&!n.hidden);smallPreview.events.load();button('Play animation').events.click();const unretained=elements(cards).find(n=>n.tag==='img'&&!n.hidden);unretained.events.load();
  assert.ok(elements(cards).some(n=>n.textContent==='Playing animation.'),'unretained content still plays completely');
  button('Back to results').events.click();assert.equal(unretained.src,'','over-budget or unknown-frame animation is not retained');assert.equal(smallPreview.src,ready.preview_url,'small preview remains eligible');
}

// A single artwork (pixiv_show_artwork) opens the detail page directly; there are no results to return to.
freshResult({...result,structuredContent:{...result.structuredContent,records:[record]}});
assert.deepEqual(latest('tools/call').params,{name:'pixiv_illust_detail',arguments:{illust_id:42}});assert.equal(button('Back to results'),undefined);
// A single record without a valid ID keeps its card instead of a detail page with no way back.
const callsBeforeInvalid=sent.filter(x=>x.message.method==='tools/call').length;
freshResult({structuredContent:{records:[{...record,id:'not-a-number'}]}});
assert.ok(cards.children.some(n=>n.className==='artwork-card'));assert.equal(sent.filter(x=>x.message.method==='tools/call').length,callsBeforeInvalid,'no detail request for an invalid ID');
// Static pixiv_artwork_media results reuse the same detail page instead of a text-heavy media view.
freshResult({structuredContent:{illust_id:42,title:'Static',pages:[{page:1,content_index:0,mime_type:'image/png',size:8}]},content:[{type:'image',mimeType:'image/png',data:'iVBORw0KGgo='}]});
assert.deepEqual(latest('tools/call').params,{name:'pixiv_illust_detail',arguments:{illust_id:42}});
// Caption navigation must not inherit executable URLs or upstream attributes.
freshResult(result);button(record.title).events.click();
const captionRequest=latest('tools/call');
receive({id:captionRequest.id,result:{structuredContent:{records:[{...record,caption:'<a href="https://example.test/artist?a=1&amp;b=2" onclick="evil()">Artist &amp; work</a><br><a href="javascript:alert(1)">unsafe</a><a href="https://user:password@example.test/">credentials</a>'}]}}});await flush();
const captionLinks=elements(cards).filter(n=>n.tag==='a'&&n.className==='caption-link');
assert.equal(captionLinks.length,1,'caption cannot turn script URLs or credential-bearing URLs into navigation');
assert.equal(captionLinks[0].textContent,'Artist & work');assert.equal(captionLinks[0].getAttribute('onclick'),undefined,'upstream event attributes are discarded');
captionLinks[0].events.click({preventDefault(){}});assert.deepEqual(latest('ui/open-link').params,{url:'https://example.test/artist?a=1&b=2'});receive({id:latest('ui/open-link').id,result:{}});await flush();
// A follow-up is one user action for the entire app, not one action per tag.
freshResult(result);button(record.title).click();
receive({id:latest('tools/call').id,result:{structuredContent:{records:[{...record,tags:[{name:'tag'},{name:'other'}]}]}}});await flush();
const searchTags=elements(cards).filter(n=>n.tag==='button'&&n.className==='tag');
const messagesBefore=sent.filter(x=>x.message.method==='ui/message').length;
searchTags[0].click();assert.ok(searchTags.every(n=>n.disabled),'all tags lock synchronously');
searchTags[1].click();assert.equal(sent.filter(x=>x.message.method==='ui/message').length,messagesBefore+1,'another tag cannot start a concurrent follow-up');
receive({id:latest('ui/message').id,result:{isError:true}});await flush();assert.ok(searchTags.every(n=>!n.disabled),'rejection restores all tags');
searchTags[1].click();receive({id:latest('ui/message').id,result:{}});await flush();
assert.ok(searchTags.every(n=>n.disabled),'message acceptance is not agent completion');
button('Back to results').click();button(record.title).click();await flush();
assert.ok(elements(cards).filter(n=>n.tag==='button'&&n.className==='tag').every(n=>n.disabled),'navigation cannot bypass the accepted follow-up lock');
for(const f of listeners.pagehide)f();assert.ok(revoked.every(u=>u.startsWith('blob:')));console.log('Gallery mock: on-demand proxy images, layout, retry, stale responses, animation');
