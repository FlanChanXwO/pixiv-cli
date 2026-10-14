
"use strict";
(() => {
  const status = document.getElementById("status");
  const cards = document.getElementById("cards");
  const pending = new Map();
  const urls = new Set();
  const proxyImages = new Set();
  const tagRows = new Set();
  const animationCleanups = new Set();
  const sessionCache = new Map();
  const cacheCapacity = 256 * 1024 * 1024;
  let cacheBytes = 0;
  function cached(key) {
    const entry = sessionCache.get(key);
    if (entry) { sessionCache.delete(key); sessionCache.set(key, entry); }
    return entry;
  }
  function remember(key, value, bytes, release = () => {}) {
    forget(key);
    if (!Number.isSafeInteger(bytes) || bytes <= 0 || bytes > cacheCapacity) return;
    while (cacheBytes + bytes > cacheCapacity) forget(sessionCache.keys().next().value);
    sessionCache.set(key, {value, bytes, release}); cacheBytes += bytes;
  }
  function forget(key) {
    const entry = sessionCache.get(key);
    if (!entry) return;
    sessionCache.delete(key); cacheBytes -= entry.bytes; entry.release();
  }
  function clearCache() { for (const key of sessionCache.keys()) forget(key); }
  let serial = 0, view = 0, ready = false, disposed = false, lastToolResult;
  let hostContext = {};
  let hostOrigin = "*", serverTools = false, downloadFiles = false, observer, resize;
  const send = message => window.parent.postMessage({jsonrpc: "2.0", ...message}, hostOrigin);
  function request(method, params, epoch) {
    return new Promise((resolve, reject) => {
      if (disposed) { reject(new Error("Gallery closed")); return; }
      const id = ++serial;
      pending.set(id, {resolve, reject, epoch});
      send({id, method, params});
    });
  }
  const dimension = value => typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;
  function applyContext(update) {
    if (!update || typeof update !== "object") return;
    hostContext = {...hostContext, ...update};
    const style = document.documentElement.style;
    style.colorScheme = ["light", "dark"].includes(hostContext.theme) ? hostContext.theme : "light dark";
    const dims = hostContext.containerDimensions ?? {};
    for (const key of ["width", "height", "maxWidth", "maxHeight"]) {
      const value = dimension(dims[key]);
      style[key] = value === undefined ? "" : value + "px";
    }
    style.overflow = "auto";
  }
  function notifySize() {
    if (disposed) return;
    for (const row of tagRows) row.update();
    const dims = hostContext.containerDimensions ?? {};
    const width = dimension(dims.width) ?? Math.min(document.documentElement.scrollWidth, dimension(dims.maxWidth) ?? Infinity);
    const height = dimension(dims.height) ?? Math.min(document.body.scrollHeight, dimension(dims.maxHeight) ?? Infinity);
    send({method: "ui/notifications/size-changed", params: {width, height}});
  }
  function clearView() {
    view++;
    for (const cleanup of animationCleanups) cleanup();
    animationCleanups.clear();
    observer?.disconnect();
    for (const [id, call] of pending) {
      if (call.epoch !== undefined) {
        pending.delete(id);
        send({method: "notifications/cancelled", params: {requestId: id, reason: "Gallery view changed"}});
        call.reject(new Error("View changed"));
      }
    }
    for (const image of proxyImages) image.removeAttribute("src");
    proxyImages.clear();
    for (const url of urls) URL.revokeObjectURL(url);
    urls.clear();
    for (const row of tagRows) resize?.unobserve(row.list);
    tagRows.clear();
    cards.replaceChildren();
  }
  function text(tag, value, parent, className) {
    const node = document.createElement(tag);
    node.textContent = String(value ?? "");
    if (className) node.className = className;
    parent.append(node);
    return node;
  }
  const iconPaths = {
    previous: "m15 18-6-6 6-6",
    next: "m9 6 6 6-6 6",
    back: "m15 18-6-6 6-6M9 12h12",
    download: "M12 3v12m-5-5 5 5 5-5M5 16v4h14v-4",
    refresh: "M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-2l2 2M4 17l2 2a7 7 0 0 0 12-2",
    bookmark: "M6 4h12v17l-6-4-6 4V4Z",
    remove: "M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14M10 11v6m4-6v6",
    play: "m9 5 11 7-11 7V5Z",
    image: "M3 3h18v18H3V3Zm0 14 6-6 5 5 3-3 4 4M8 7h.01",
    loader: "M20 12a8 8 0 1 1-8-8",
    pages: "M8 3h13v13M3 8h13v13H3V8Z",
    user: "M16 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0ZM4 21v-2a8 8 0 0 1 16 0v2",
    eye: "M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z",
    size: "M3 9V3h6m6 0h6v6M3 15v6h6m6 0h6v-6",
    calendar: "M4 5h16v16H4V5Zm0 5h16M8 3v4m8-4v4",
    id: "M9 3 7 21m10-18-2 18M3 9h18M3 15h18"
  };
  function icon(name) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    for (const [key, value] of Object.entries({viewBox:"0 0 24 24", fill:"none", stroke:"currentColor", "stroke-width":"1.8", "stroke-linecap":"round", "stroke-linejoin":"round", "aria-hidden":"true", focusable:"false"})) svg.setAttribute(key, value);
    const path = document.createElementNS("http://www.w3.org/2000/svg", "path"); path.setAttribute("d", iconPaths[name]); svg.append(path);
    return svg;
  }
  function iconAction(label, name, parent, handler, iconOnly = false) {
    const button = action(iconOnly ? "" : label, parent, handler);
    button.title = label; button.setAttribute("aria-label", label); button.append(icon(name));
    if (iconOnly) button.className = "icon-button";
    return button;
  }
  function ratingBadge(record, parent) {
    const label = record.x_restrict === 1 ? "R-18" : record.x_restrict === 2 ? "R-18G" : "";
    if (label) text("span", label, parent, "rating-badge");
  }
  function mediaBadges(record, frame) {
    const badges = document.createElement("div"); badges.className = "media-badges"; frame.append(badges);
    ratingBadge(record, badges);
    if (Number.isSafeInteger(record.page_count) && record.page_count > 1) {
      const badge = text("span", String(record.page_count), badges, "page-badge");
      badge.setAttribute("aria-label", record.page_count + " images"); badge.append(icon("pages"));
    }
  }
  function artworkAuthor(record, parent) {
    if (typeof record.user?.name === "string" && record.user.name) {
      const id = String(record.user.id ?? "");
      const validID = /^[1-9][0-9]*$/.test(id) && (typeof record.user.id === "string" || Number.isSafeInteger(record.user.id));
      const author = validID ? action(record.user.name, parent, async () => {
        if (author.disabled) return;
        author.disabled = true;
        const epoch = view;
        try { await request("ui/open-link", {url:"https://www.pixiv.net/users/" + id}, epoch); }
        catch (_) { if (epoch === view && !disposed) status.textContent = "Author page could not be opened."; }
        finally { author.disabled = false; }
      }) : text("p", record.user.name, parent);
      author.className = "artwork-author"; author.append(icon("user"));
      if (validID) { author.title = "Open author page"; author.setAttribute("aria-label", "Open author page — " + record.user.name); }
    }
  }
  function pageDimensions(record, page) {
    const item = Array.isArray(record.pages) ? record.pages.find(item => item?.page_index === page - 1) : undefined;
    const valid = item => Number.isSafeInteger(item?.width) && item.width > 0 && Number.isSafeInteger(item?.height) && item.height > 0;
    const source = valid(item) ? item : valid(record) ? record : undefined;
    return source ? source.width + " × " + source.height : "";
  }
  function artworkMetadata(record, parent) {
    const list = document.createElement("dl"); list.className = "artwork-metadata"; parent.append(list);
    const row = (label, value, name) => {
      const group = document.createElement("div"); list.append(group);
      const term = text("dt", label, group); term.append(icon(name));
      return text("dd", value, group, "metadata-value");
    };
    for (const [key, label, name] of [["total_views", "Views", "eye"], ["total_bookmarks", "Bookmarks", "bookmark"]]) {
      if (Number.isSafeInteger(record[key]) && record[key] >= 0) row(label, String(record[key]), name);
    }
    if (typeof record.id === "string" && /^[1-9][0-9]*$/.test(record.id)) row("Artwork ID", record.id, "id");
    const dimensions = pageDimensions(record, 1);
    let size = dimensions ? row("Dimensions", dimensions, "size") : undefined;
    if (typeof record.published_at === "string" && record.published_at !== "0001-01-01T00:00:00Z") {
      const date = new Date(record.published_at);
      if (Number.isFinite(date.getTime())) {
        const value = row("Published", "", "calendar");
        const time = text("time", new Intl.DateTimeFormat(undefined, {year:"numeric", month:"short", day:"numeric"}).format(date), value);
        time.dateTime = record.published_at; time.title = record.published_at;
      }
    }
    return page => { const value = pageDimensions(record, page); if (!size && value) size = row("Dimensions", value, "size"); if (size) size.textContent = value; };
  }
  function tagChips(tags, parent, collapsible = false) {
    if (!Array.isArray(tags) || !tags.length) return;
    const group = document.createElement("div"); group.className = "tags"; parent.append(group);
    const list = document.createElement("div"); list.className = "tag-list"; group.append(list);
    for (const tag of tags) {
      if (typeof tag?.name === "string" && tag.name) text("span", tag.name, list, "tag");
    }
    if (!collapsible || !list.children.length) return;
    group.className = "tags collapsed";
    let expanded = false;
    const toggle = action("...", group, () => {
      expanded = !expanded;
      group.className = expanded ? "tags expanded" : "tags collapsed";
      const label = expanded ? "Show fewer tags" : "Show all tags";
      toggle.title = label; toggle.setAttribute("aria-label", label);
      toggle.setAttribute("aria-expanded", String(expanded));
      row.update();
    });
    toggle.title = "Show all tags"; toggle.setAttribute("aria-label", "Show all tags");
    toggle.className = "tag-toggle"; toggle.setAttribute("aria-expanded", "false");
    const row = {list, update: () => { toggle.hidden = !expanded && list.scrollWidth <= group.clientWidth; }};
    tagRows.add(row); resize?.observe(list); row.update();
  }
  function proxyImage(record, parent, initial, epoch, page = 1, quality = "thumbnail", open, openOriginal, onReady) {
    const frame = document.createElement("div"); frame.className = "media-frame"; parent.append(frame);
    if (quality !== "thumbnail") mediaBadges(record, frame);
    const key = "image:" + record.id + ":" + page + ":" + quality;
    const retained = cached(key);
    const image = retained?.value ?? document.createElement("img"); image.alt = String(record.title || "Artwork") + " — page " + page; image.hidden = !retained; image.draggable = false; image.referrerPolicy = "no-referrer"; proxyImages.add(image);
    if (open) { const link = action("", frame, open); link.className = "media-open"; link.title = "Open artwork"; link.setAttribute?.("aria-label", "Open " + String(record.title || "artwork")); link.append(image); } else frame.append(image);
    const overlay = document.createElement("div"); overlay.className = "media-state"; frame.append(overlay);
    const message = text("span", "", overlay); message.setAttribute?.("role", "status");
    const spinner = icon("loader"); spinner.setAttribute("class", "spinner"); overlay.append(spinner);
    const retry = iconAction("Retry image", "refresh", overlay, () => load(true)); retry.hidden = true; retry.disabled = !serverTools;
    let busy = false, started = Boolean(retained), active = true;
    overlay.hidden = Boolean(retained);
    const failed = () => { if (!active || epoch !== view || disposed) return; busy = false; forget(key); image.hidden = true; overlay.hidden = false; spinner.style.display = "none"; message.className = ""; message.textContent = "Image unavailable"; retry.hidden = false; retry.disabled = !serverTools; };
    const loaded = () => {
      if (!active || epoch !== view || disposed) return;
      busy = false; image.hidden = false; overlay.hidden = true; retry.hidden = true;
      // Include decoded pixels and an encoded-image allowance; browser/GPU overhead is outside this estimate.
      const bytes = image.naturalWidth * image.naturalHeight * 8;
      remember(key, image, bytes, () => { if (!proxyImages.has(image)) image.removeAttribute("src"); });
      onReady?.();
    };
    image.addEventListener("load", loaded);
    image.addEventListener("error", failed);
    const originalKey = event => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); openOriginal(); } };
    if (openOriginal) {
      image.tabIndex = 0; image.setAttribute("role", "button"); image.setAttribute("aria-label", "View original — " + String(record.title || "Artwork") + " — page " + page); image.title = "View original";
      image.addEventListener("click", openOriginal); image.addEventListener("dblclick", openOriginal); image.addEventListener("keydown", originalKey);
    }
    const assign = result => {
      const output = result?.structuredContent;
      const item = output?.images?.find(p => Number(p.illust_id) === Number(record.id) && p.page === page);
      // Only the canonical origin returned by our media tool is allowed; CSP also pins it.
      if (!item || typeof output.media_origin !== "string" || !/^https?:\/\/[^/?#@]+$/.test(output.media_origin) || typeof item.url !== "string" || !item.url.startsWith(output.media_origin + "/") || !item.url.includes("/media/") || /[?#]/.test(item.url)) throw new Error("Invalid image URL");
      image.src = item.url;
    };
    const load = async renew => {
      if (retained && !renew && active) { onReady?.(); return; }
      if (!active || busy || epoch !== view || disposed || (started && !renew)) return;
      started = true; busy = true; retry.hidden = true; overlay.hidden = false; spinner.style.display = ""; message.className = "sr-only"; message.textContent = "Loading image…";
      try {
        if (!renew && typeof initial === "function") { const response = await initial(); if (!active || epoch !== view || disposed) return; assign(response); }
        else if (!renew && initial?.structuredContent?.images?.some(p => Number(p.illust_id) === Number(record.id) && p.page === page)) assign(initial);
        else { if (!serverTools) throw new Error("Host does not allow tool calls"); const response = await request("tools/call", {name:"pixiv_show_gallery", arguments:{illust_ids:[Number(record.id)],page,quality}}, epoch); if (!active || epoch !== view || disposed) return; assign(response); }
      } catch (_) { failed(); }
    };
    const dispose = () => {
      if (!active) return;
      active = false; image.removeEventListener("load", loaded); image.removeEventListener("error", failed);
      if (openOriginal) { image.removeEventListener("click", openOriginal); image.removeEventListener("dblclick", openOriginal); image.removeEventListener("keydown", originalKey); }
      image.remove(); proxyImages.delete(image);
      if (sessionCache.get(key)?.value !== image) image.removeAttribute("src");
      animationCleanups.delete(dispose);
    };
    animationCleanups.add(dispose);
    return {load: () => load(false), frame, image, dispose, page};
  }
  function action(label, parent, handler) {
    const button = text("button", label, parent);
    button.type = "button";
    button.addEventListener("click", handler);
    return button;
  }
  function imageURL(content, page) {
    if (!content || content.type !== "image" || typeof content.data !== "string" || !["image/png","image/jpeg","image/webp","image/gif","image/bmp"].includes(content.mimeType)) throw new Error("Invalid image content");
    const bytes = Uint8Array.from(atob(content.data), char => char.charCodeAt(0));
    if (page && (bytes.byteLength !== page.size || content.mimeType !== page.mime_type)) throw new Error("Invalid image metadata");
    const url = URL.createObjectURL(new Blob([bytes], {type: content.mimeType}));
    urls.add(url);
    return url;
  }
  function downloadControl(content, filename, parent, epoch, failed) {
    if (!downloadFiles) {
      const link = text("a", "", parent, "icon-button browser-download");
      link.setAttribute("aria-label", "Download " + filename); link.title = "Download " + filename;
      link.append(icon("download")); link.href = content.uri + "?download=1";
      link.target = "_blank"; link.rel = "noreferrer";
      text("p", "Save using your browser. Browser download restrictions still apply.", parent);
      return;
    }
    const message = text("p", "Download uses the host confirmation flow.", parent);
    let busy = false;
    const button = iconAction("Download " + filename, "download", parent, async () => {
      if (!downloadFiles || busy || epoch !== view || !parent.contains(button)) return;
      busy = true; button.disabled = true; message.textContent = "Waiting for host download confirmation…";
      try {
        const result = await request("ui/download-file", {contents: [content]}, epoch);
        if (epoch !== view || disposed) return;
        if (!result || result.isError) throw new Error("Download not confirmed");
        message.textContent = "Host confirmed download. File location and naming are controlled by the host.";
      } catch (_) { if (epoch === view && parent.contains(button)) { message.textContent = "Download not confirmed (denied, cancelled or failed)."; failed(); } }
      finally { busy = false; button.disabled = !downloadFiles; }
    });
    button.disabled = !downloadFiles;
  }
  function animationControls(id, artwork, section, epoch, initialFormat = "gif") {
    const stateKey = "animation-state:" + id;
    const retained = cached(stateKey)?.value;
    const mediaKey = (format, url) => "animation-image:" + id + ":" + format + ":" + url;
    const label = text("label", "Animation format ", section);
    const format = document.createElement("select");
    for (const value of ["gif", "apng"]) { const option = text("option", value, format); option.value = value; }
    format.value = ["gif", "apng"].includes(retained?.format) ? retained.format : initialFormat; label.append(format);
    const message = text("p", "Prepare animation and a separate static preview.", section);
    const media = document.createElement("div"); section.append(media);
    let reading = false, handle, timer, generation = 0, images = [];
    const releaseHandle = old => {
      if (old && !disposed) request("tools/call", {name:"pixiv_gallery_animation", arguments:{action:"release",handle:old}}).catch(() => {
        if (epoch === view && !disposed) message.textContent = "Subscription release was not confirmed. It will expire automatically.";
      });
    };
    const release = () => {
      generation++; clearTimeout(timer); timer = undefined;
      const old = handle; handle = undefined; reading = false;
      for (const item of images) {
        item.image.removeEventListener("load", item.loaded); item.image.removeEventListener("error", item.failed);
        proxyImages.delete(item.image);
        if (sessionCache.get(item.key)?.value !== item.image) item.image.removeAttribute("src");
      }
      images = [];
      releaseHandle(old);
    };
    animationCleanups.add(release);
    const current = token => token === generation && epoch === view && !disposed;
    const failed = token => {
      if (!current(token)) return;
      release(); forget(stateKey); media.replaceChildren(); load.disabled = false; load.hidden = false;
      message.textContent = "Animation read failed or incomplete. Retry explicitly.";
    };
    const validURL = (url, origin) => typeof origin === "string" && /^https?:\/\/[^/?#@]+$/.test(origin) && typeof url === "string" && url.startsWith(origin + "/") && url.includes("/media/animation/") && !/[?#]/.test(url);
    const receiveState = (result, token, selectedFormat) => {
      const output = result.structuredContent;
      if (!current(token)) {
        if (typeof output?.handle === "string") releaseHandle(output.handle);
        return;
      }
      if (typeof output?.handle === "string" && output.handle) {
        if (handle && handle !== output.handle) throw new Error("Different subscription");
        handle = output.handle;
      }
      if (result.isError || !output || !handle || !["queued","downloading","encoding","ready"].includes(output.state)) throw new Error("Animation unavailable");
      if (output.state !== "ready") {
        message.textContent = {queued:"Animation queued…",downloading:"Downloading animation frames…",encoding:"Encoding animation…"}[output.state];
        timer = setTimeout(async () => {
          timer = undefined;
          if (!current(token) || !handle) return;
          try { receiveState(await request("tools/call", {name:"pixiv_gallery_animation",arguments:{action:"status",handle}}, epoch), token, selectedFormat); }
          catch (_) { failed(token); }
        }, 2000);
        return;
      }
      if (!validURL(output.animation_url, output.media_origin) || !validURL(output.preview_url, output.media_origin) || output.animation_url === output.preview_url || output.mime_type !== "image/" + selectedFormat || !Number.isSafeInteger(output.size) || output.size <= 0 || typeof output.filename !== "string") throw new Error("Invalid animation delivery");
      media.replaceChildren();
      const failure = "Animation or preview delivery failed. Retry explicitly.";
      images = [
        {url:output.preview_url, loading:"Loading static preview…", ready:"Static preview.", state:"idle"},
        {url:output.animation_url, loading:"Loading animation…", ready:"Playing animation.", state:"idle"}
      ];
      let selected;
      const show = item => {
        if (!current(token)) return;
        selected = item;
        for (const other of images) other.image.hidden = other !== item;
        // Keep loaded media in this view: assigning src again would trigger another no-store download.
        if (item.state === "idle") { item.state = "loading"; item.image.src = item.url; }
        message.textContent = item.state === "ready" ? item.ready : item.state === "failed" ? failure : item.loading;
      };
      for (const item of images) {
        item.key = mediaKey(selectedFormat, item.url);
        const image = cached(item.key)?.value ?? document.createElement("img"); item.image = image;
        if (sessionCache.get(item.key)?.value === image) item.state = "ready";
        image.alt = String(artwork.title); image.hidden = true; image.referrerPolicy = "no-referrer";
        proxyImages.add(image); media.append(image);
        item.loaded = () => {
          if (!current(token)) return;
          item.state = "ready";
          // Count every decoded animation frame; unknown frame counts are not retained.
          const pixels = Number(image.naturalWidth || artwork.width) * Number(image.naturalHeight || artwork.height);
          const frames = result._meta?.["pixiv/animationFrameCount"];
          let bytes = pixels * 8;
          if (item.url === output.animation_url) bytes = Number.isSafeInteger(frames) && frames > 0 ? output.size + pixels * 4 * frames : NaN;
          remember(item.key, image, bytes, () => { if (!proxyImages.has(image)) image.removeAttribute("src"); });
          if (selected === item) message.textContent = item.ready;
        };
        item.failed = () => {
          if (!current(token)) return;
          item.state = "failed"; forget(item.key); load.hidden = false;
          if (selected === item) message.textContent = failure;
        };
        image.addEventListener("load", item.loaded);
        image.addEventListener("error", item.failed);
      }
      iconAction("Play animation", "play", media, () => show(images[1]));
      iconAction("Show preview", "image", media, () => show(images[0]));
      show(images[0]);
      downloadControl({type:"resource_link",uri:output.animation_url,name:output.filename,mimeType:output.mime_type,size:output.size}, output.filename, media, epoch, () => { if (current(token)) load.hidden = false; });
      remember(stateKey, {format:selectedFormat, animation:output.animation_url, preview:output.preview_url}, (output.animation_url.length + output.preview_url.length) * 2 + 16);
      reading = false; load.disabled = false; load.hidden = true;
    };
    const load = action("Load animation", section, async () => {
      if (reading || epoch !== view || disposed) return;
      if (!["gif","apng"].includes(format.value)) { message.textContent = "Invalid animation format."; return; }
      release(); media.replaceChildren(); const token = generation, selectedFormat = format.value;
      reading = true; load.disabled = true; message.textContent = "Preparing animation…";
      // Keep prepare responses reachable after navigation so late handles can be released.
      try { receiveState(await request("tools/call", {name:"pixiv_gallery_animation",arguments:{action:"prepare",illust_id:id,format:selectedFormat}}), token, selectedFormat); }
      catch (_) { failed(token); }
    });
    format.addEventListener("change", () => { release(); media.replaceChildren(); load.disabled = false; load.hidden = false; message.textContent = "Load the selected animation format."; });
    if (retained && [retained.preview, retained.animation].some(url => cached(mediaKey(retained.format, url)))) load.click();
    return load;
  }
  async function originalContent(id, pageNumber, epoch, artwork) {
    const key = "original:" + id + ":" + pageNumber;
    const retained = cached(key);
    if (retained) return retained.value;
    const response = await request("tools/call", {name:"pixiv_artwork_media",arguments:{illust_id:id,pages:[pageNumber],quality:"original"}}, epoch);
    if (epoch !== view || disposed) throw new Error("View changed");
    const page = response.structuredContent?.pages?.find(p => p.page === pageNumber);
    const content = page && response.content?.[page.content_index];
    if (response.isError || content?.type !== "image" || typeof content.data !== "string" || !["image/png","image/jpeg","image/webp","image/gif","image/bmp"].includes(content.mimeType)) throw new Error("Missing bytes");
    const size = atob(content.data).length;
    if ((page.size !== undefined && page.size !== size) || (page.mime_type !== undefined && page.mime_type !== content.mimeType)) throw new Error("Invalid image metadata");
    const dimensions = artwork.pages?.find(p => p.page_index === pageNumber - 1) ?? artwork;
    remember(key, content, content.data.length * 2 + size + (Number(dimensions.width) * Number(dimensions.height) * 4 || 0));
    return content;
  }
  async function downloadOriginal(id, page, epoch, artwork, loaded) {
    const content = loaded ?? await originalContent(id, page, epoch, artwork);
    if (epoch !== view || disposed) return;
    const saved = await request("ui/download-file", {contents:[{type:"resource",resource:{uri:"urn:pixiv:artwork:" + id + ":page:" + page,mimeType:content.mimeType,blob:content.data}}]}, epoch);
    if (!saved || saved.isError) throw new Error("Download rejected");
  }
  let originalOpen = false;
  function originalViewer(id, page, artwork, epoch) {
    if (originalOpen || epoch !== view || disposed) return;
    originalOpen = true;
    const dialog = document.createElement("dialog"); dialog.className = "original-viewer"; dialog.setAttribute("aria-label", "Original artwork — page " + page); cards.append(dialog);
    const toolbar = document.createElement("div"); toolbar.className = "original-toolbar"; dialog.append(toolbar);
    let url, original, closed = false;
    const cleanup = () => {
      if (closed) return; closed = true; originalOpen = false;
      image.removeAttribute("src"); dialog.close(); dialog.remove();
      if (url) { URL.revokeObjectURL(url); urls.delete(url); }
      animationCleanups.delete(cleanup);
    };
    iconAction("Close original", "back", toolbar, cleanup, true);
    const zoom = iconAction("Actual size", "size", toolbar, () => {
      const actual = image.className !== "original-image actual-size";
      image.className = actual ? "original-image actual-size" : "original-image";
      zoom.setAttribute("aria-pressed", String(actual));
    }); zoom.setAttribute("aria-pressed", "false"); zoom.disabled = true;
    const message = text("p", "Loading original…", dialog); message.setAttribute("role", "status");
    if (downloadFiles) {
      const download = iconAction("Download viewed original", "download", toolbar, async () => {
        download.disabled = true;
        try { await downloadOriginal(id, page, epoch, artwork, original); if (!closed) message.textContent = "Download requested."; }
        catch (_) { if (!closed) message.textContent = "Download failed. Retry explicitly."; }
        finally { download.disabled = false; }
      }, true); download.disabled = true;
      dialog.enableDownload = () => { download.disabled = false; };
    } else {
      // 宿主外链校验拒绝 Blob；以同一页原图凭证交给浏览器保存。
      const download = text("a", "", toolbar, "icon-button browser-download");
      download.setAttribute("aria-label", "Download viewed original"); download.title = "Download viewed original";
      download.append(icon("download")); download.hidden = true;
      dialog.enableDownload = async () => {
        message.textContent = "Preparing original download…";
        try {
          const result = await request("tools/call", {name:"pixiv_show_gallery",arguments:{illust_ids:[Number(id)],page,quality:"original"}}, epoch);
          if (closed || epoch !== view || disposed) return;
          const output = result.structuredContent;
          const item = output?.images?.find(item => Number(item.illust_id) === Number(id) && item.page === page);
          if (result.isError || !item || typeof output.media_origin !== "string" || !/^https?:\/\/[^/?#@]+$/.test(output.media_origin) || typeof item.url !== "string" || !item.url.startsWith(output.media_origin + "/") || !item.url.includes("/media/") || /[?#]/.test(item.url)) throw new Error("Invalid original delivery");
          download.href = item.url + "?download=1"; download.target = "_blank"; download.rel = "noreferrer"; download.hidden = false;
          message.textContent = "Save using your browser. Browser download restrictions still apply.";
        } catch (_) { if (!closed && epoch === view && !disposed) message.textContent = "Original download unavailable. Close and reopen to retry."; }
      };
    }
    const surface = document.createElement("div"); surface.className = "original-surface"; dialog.append(surface);
    const image = document.createElement("img"); image.className = "original-image"; image.alt = String(artwork.title || "Artwork") + " — original page " + page; image.hidden = true; surface.append(image);
    image.addEventListener("load", () => {
      if (closed) return;
      remember("original:" + id + ":" + page, original, original.data.length * 2 + atob(original.data).length + image.naturalWidth * image.naturalHeight * 4);
      image.hidden = false; zoom.disabled = false; message.textContent = ""; dialog.enableDownload?.();
    });
    image.addEventListener("error", () => { if (!closed) { forget("original:" + id + ":" + page); message.textContent = "Original unavailable. Close and reopen to retry."; } });
    image.addEventListener("dblclick", () => zoom.click());
    dialog.addEventListener("cancel", event => { event.preventDefault(); cleanup(); });
    animationCleanups.add(cleanup); dialog.showModal();
    originalContent(id, page, epoch, artwork).then(content => {
      if (closed || epoch !== view || disposed) return;
      original = content; url = imageURL(content); image.src = url;
    }).catch(() => { if (!closed) message.textContent = "Original unavailable. Close and reopen to retry."; });
  }
  async function openDetail(record, previous) {
    clearView();
    const epoch = view;
    const id = Number(record.id);
    iconAction("Back to results", "back", cards, () => render(previous), true);
    if (!serverTools || !Number.isSafeInteger(id) || id <= 0) { status.textContent = "Artwork detail unavailable."; return; }
    status.textContent = "Loading artwork…";
    try {
      const metadataKey = "detail:" + id;
      const result = cached(metadataKey)?.value ?? await request("tools/call", {name: "pixiv_illust_detail", arguments: {illust_id: id}}, epoch);
      if (epoch !== view || disposed) return;
      const artwork = result.structuredContent?.records?.find(item => item && Number(item.id) === id);
      if (result.isError || !artwork) throw new Error("Artwork unavailable");
      remember(metadataKey, result, JSON.stringify(result).length * 2);
      status.textContent = "";
      const section = document.createElement("article"); section.className = "artwork-detail"; cards.append(section);
      const mediaColumn = document.createElement("div"); mediaColumn.className = "detail-media"; section.append(mediaColumn);
      const info = document.createElement("div"); info.className = "detail-info"; section.append(info);
      text("h2", artwork.title, info);
      artworkAuthor(artwork, info);
      if (artwork.type === "ugoira") ratingBadge(artwork, info);
      tagChips(artwork.tags, info);
      // Decode Pixiv numeric entities into text; never interpret caption markup.
      if (artwork.caption) {
        const caption = String(artwork.caption).replace(/<br\s*\/?>/gi, "\n").replace(/&#(x[0-9a-f]+|[0-9]+);/gi, (entity, digits) => {
          const code = /^x/i.test(digits) ? Number.parseInt(digits.slice(1), 16) : Number(digits);
          return code > 0 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff) ? String.fromCodePoint(code) : entity;
        });
        text("p", caption, info, "caption");
      }
      const updateDimensions = artworkMetadata(artwork, info);
      const detailControls = document.createElement("div"); detailControls.className = "detail-controls";
      if (artwork.type === "ugoira") {
        animationControls(id, artwork, mediaColumn, epoch);
      } else {
        const total = artwork.page_count;
        if (!Number.isSafeInteger(total) || total < 1) throw new Error("Invalid page count");
        const viewer = document.createElement("div"); viewer.className = "page-viewer"; mediaColumn.append(viewer);
        const media = document.createElement("div"); media.className = "page-media"; viewer.append(media);
        let selectedPage = 1, preview, displayed, previousPage, nextPage, progress, position;
        let swiped = false;
        // Pointer capture retargets taps to the media container; swipes must not open the original.
        media.addEventListener("click", event => { if (!swiped && !event.target?.closest("button")) originalViewer(id, selectedPage, artwork, epoch); });
        media.addEventListener("dblclick", event => {
          if (!swiped && !event.target?.closest("button")) originalViewer(id, selectedPage, artwork, epoch);
        });
        const validPage = page => Number.isSafeInteger(page) && page >= 1 && page <= total;
        const loadPage = page => {
          if (!validPage(page) || epoch !== view || disposed) return;
          selectedPage = page; remember("page:" + id, page, 8); updateDimensions(page);
          if (preview && preview !== displayed) { preview.dispose(); preview.frame.remove(); }
          if (displayed?.page === page) preview = displayed;
          else {
            const replacement = proxyImage(artwork, media, null, epoch, page, "regular", undefined, () => originalViewer(id, page, artwork, epoch), () => {
              if (selectedPage !== page || epoch !== view || disposed) return;
              if (displayed) { displayed.dispose(); displayed.frame.remove(); }
              displayed = replacement; replacement.frame.className = "media-frame";
            });
            preview = replacement;
            if (displayed) replacement.frame.className = "media-frame page-pending";
            replacement.load();
          }
          if (progress) { progress.value = String(page); position.textContent = page + " / " + total; previousPage.disabled = page === 1; nextPage.disabled = page === total; }
        };
        if (total > 1) {
          previousPage = iconAction("Previous image", "previous", viewer, () => { if (selectedPage > 1) loadPage(selectedPage - 1); }, true); previousPage.className = "icon-button page-previous";
          nextPage = iconAction("Next image", "next", viewer, () => { if (selectedPage < total) loadPage(selectedPage + 1); }, true); nextPage.className = "icon-button page-next";
          const navigation = document.createElement("div"); navigation.className = "page-navigation"; mediaColumn.append(navigation);
          progress = document.createElement("input"); progress.type = "range"; progress.min = "1"; progress.max = String(total); progress.step = "1"; progress.setAttribute("aria-label", "Image progress"); navigation.append(progress);
          position = text("output", "", navigation, "page-position");
          progress.addEventListener("input", () => { const page = Number(progress.value); if (validPage(page)) position.textContent = page + " / " + total; });
          progress.addEventListener("change", () => { const page = Number(progress.value); if (validPage(page) && page !== selectedPage) loadPage(page); });
          let gesture;
          media.addEventListener("pointerdown", event => { if (event.target?.closest("button")) return; if (event.isPrimary !== false && event.button === 0) { swiped = false; gesture = {id:event.pointerId, x:event.clientX, y:event.clientY}; media.setPointerCapture?.(event.pointerId); } });
          media.addEventListener("pointercancel", () => { gesture = undefined; });
          media.addEventListener("pointerup", event => {
            const start = gesture; gesture = undefined;
            if (!start || start.id !== event.pointerId) return;
            const dx = event.clientX - start.x, dy = event.clientY - start.y;
            // A full touch target of horizontal travel distinguishes swipes from taps; pan-y preserves chat scrolling.
            if (Math.abs(dx) >= 44 && Math.abs(dx) > Math.abs(dy) * 1.5) { swiped = true; event.preventDefault(); const page = selectedPage + (dx < 0 ? 1 : -1); if (validPage(page)) loadPage(page); }
          });
        }
        const retainedPage = cached("page:" + id)?.value;
        loadPage(validPage(retainedPage) ? retainedPage : 1);
        if (downloadFiles && serverTools) {
          const message = text("p", "", detailControls);
          const download = iconAction("Download original page", "download", detailControls, async () => {
            if (download.disabled) return; download.disabled = true; const pageNumber = selectedPage;
            try { await downloadOriginal(id, pageNumber, epoch, artwork); if (epoch !== view || disposed) return; message.textContent = "Download requested."; }
            catch (_) { if (epoch === view) message.textContent = "Download failed. Retry explicitly."; } finally { download.disabled = false; }
          }, true);
        }
      }
      info.append(detailControls);
      bookmarkControls(id, info, epoch);
    } catch (_) { if (epoch === view && !disposed) status.textContent = "Artwork detail failed. Return to results and retry."; }
  }
  function bookmarkControls(id, section, epoch) {
    const group = document.createElement("div"); group.className = "bookmark-controls"; section.append(group);
    const message = text("p", "Loading bookmark state…", group, "bookmark-status"); message.setAttribute("role", "status");
    const toolbar = document.createElement("div"); toolbar.className = "bookmark-toolbar"; group.append(toolbar);
    let state, busy = false;
    const enable = () => {
      save.disabled = remove.disabled = visibility.disabled = busy || !state;
      refresh.disabled = busy;
      remove.hidden = !state?.bookmarked;
    };
    const query = async () => {
      if (busy || epoch !== view) return;
      busy = true; enable();
      try {
        const result = await request("tools/call", {name: "pixiv_bookmark_detail", arguments: {illust_id: id}}, epoch);
        if (epoch !== view || disposed) return;
        const value = result.structuredContent;
        if (result.isError || typeof value?.bookmarked !== "boolean" || (value.tags !== null && (!Array.isArray(value.tags) || !value.tags.every(tag => typeof tag === "string")))) throw new Error("Unknown bookmark state");
        state = {...value, tags: value.tags ?? []};
        if (["public", "private"].includes(value.restrict)) visibility.value = value.restrict;
        const label = value.bookmarked ? "Save bookmark" : "Bookmark";
        save.textContent = label; save.append(icon("bookmark")); save.setAttribute("aria-label", label); save.title = label;
        message.textContent = value.bookmarked ? "Bookmarked (" + value.restrict + ")" : "Not bookmarked";
      } catch (_) { if (epoch === view) { state = undefined; message.textContent = "Bookmark state unavailable. Refresh before writing."; } }
      finally { busy = false; enable(); }
    };
    const mutate = async restrict => {
      if (busy || !state || epoch !== view || (restrict && !["public", "private"].includes(restrict))) return;
      busy = true; enable();
      const args = {illust_id: id};
      if (restrict) { args.restrict = restrict; args.tags = state.tags; }
      let success = false;
      try {
        const result = await request("tools/call", {name: restrict ? "pixiv_add_bookmark" : "pixiv_remove_bookmark", arguments: args}, epoch);
        if (epoch !== view || disposed) return;
        if (result.isError) throw new Error("Bookmark operation failed");
        success = true;
      } catch (_) { if (epoch === view) { state = undefined; message.textContent = "Bookmark change not confirmed. Refresh state before retrying."; } }
      finally { busy = false; enable(); }
      if (success) await query();
    };
    const save = iconAction("Bookmark", "bookmark", toolbar, () => mutate(visibility.value)); save.className = "primary-action";
    const visibility = document.createElement("select"); visibility.setAttribute("aria-label", "Bookmark visibility"); toolbar.append(visibility);
    for (const value of ["public", "private"]) { const option = text("option", value === "public" ? "Public" : "Private", visibility); option.value = value; }
    visibility.value = "public";
    const remove = iconAction("Remove bookmark", "remove", toolbar, () => mutate(""), true);
    const refresh = iconAction("Refresh bookmark", "refresh", toolbar, query, true);
    enable(); query();
  }
  function renderMedia(result, epoch) {
    const output = result.structuredContent;
    const appContent = result._meta?.["pixiv/mediaContents"];
    const content = appContent === undefined ? result.content : appContent;
    status.textContent = result.isError ? "Tool failed or returned partial results." : "";
    if (!output.pages.length) { status.textContent = output.error || "No media available."; return; }
    for (const page of output.pages) {
      const section = text("article", "", cards, "artwork-detail");
      const media = text("div", "", section, "detail-media");
      const info = text("div", "", section, "detail-info");
      text("h2", output.title || "Artwork " + output.illust_id, info);
      const message = text("p", "Loading image…", info);
      try {
        if (!Array.isArray(content)) throw new Error("Invalid media content");
        const original = content?.[page.content_index];
        const linked = original?.type === "resource_link";
        const downloads = result._meta?.["pixiv/mediaDownloads"];
        const item = downloads?.pages?.find(item => item.page === page.page);
        const origin = downloads?.media_origin;
        const downloadURL = typeof origin === "string" && /^https?:\/\/[^/?#@]+$/.test(origin) && typeof item?.url === "string" && item.url.startsWith(origin + "/") && item.url.includes("/media/") && !/[?#]/.test(item.url) ? item.url : undefined;
        if (!Number.isSafeInteger(page.page) || page.page < 1 || !Number.isSafeInteger(page.content_index) || page.content_index < 0 || (!linked && (original?.type !== "image" || typeof original.data !== "string"))) throw new Error("Missing media");
        if (original.mimeType !== page.mime_type && !(page.mime_type === "image/apng" && original.mimeType === "image/png")) throw new Error("Invalid media type");
        if (linked) {
          if (!downloadURL || original.uri !== downloadURL || !Number.isSafeInteger(page.size) || page.size <= 0 || original.size !== page.size) throw new Error("Invalid media link");
        } else if (atob(original.data).length !== page.size) throw new Error("Incomplete media");
        const extensions = {"image/png":"png","image/jpeg":"jpg","image/webp":"webp","image/gif":"gif","image/bmp":"bmp","image/apng":"apng"};
        const extension = extensions[page.mime_type];
        if (!extension) throw new Error("Unsupported media");
        const filename = page.filename || output.illust_id + "-p" + page.page + "." + extension;
        if (typeof filename !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(filename)) throw new Error("Invalid filename");
        const animated = Number.isSafeInteger(page.preview_content_index) && page.preview_content_index >= 0;
        if (animated && (page.preview_content_index === page.content_index || content?.[page.preview_content_index]?.mimeType !== "image/png")) throw new Error("Invalid static preview");
        const images = new Map();
        let selected;
        const update = () => {
          if (epoch !== view || !selected) return;
          retry.hidden = !linked && selected.state !== "failed";
          message.textContent = selected.state === "failed" ? "Image unavailable." : selected.state === "loading" ? "Loading image…" : animated ? selected.index === page.content_index ? "Animation playing." : "Static preview." : "Image ready.";
        };
        const retry = iconAction(linked ? "Refresh animation" : "Retry image", "refresh", info, () => {
          if (!selected || epoch !== view) return;
          if (linked) {
            if (!serverTools) return;
            // 历史签名地址无法原地恢复；复用现有准备与释放流程重新授权。
            for (const entry of images.values()) { entry.image.removeAttribute("src"); proxyImages.delete(entry.image); }
            media.replaceChildren(); info.replaceChildren();
            text("h2", output.title || "Artwork " + output.illust_id, info);
            animationControls(output.illust_id, output, media, epoch, extension).click();
            return;
          }
          selected.state = "loading"; selected.image.removeAttribute("src"); selected.image.src = selected.url; update();
        });
        retry.hidden = !linked;
        retry.disabled = linked && !serverTools;
        const show = index => {
          if (epoch !== view) return;
          try {
            let entry = images.get(index);
            if (!entry) {
              const item = content?.[index];
              const url = linked && index === page.content_index ? original.uri : imageURL(item, index === page.content_index ? {size:page.size,mime_type:original.mimeType} : undefined);
              const image = text("img", "", media);
              image.referrerPolicy = "no-referrer";
              image.alt = (output.title || "Artwork " + output.illust_id) + " — " + filename;
              entry = {image, url, index, state:"loading"}; images.set(index, entry); proxyImages.add(image);
              const loaded = () => { entry.state = "ready"; if (selected === entry) update(); };
              const failed = () => { entry.state = "failed"; if (selected === entry) update(); };
              image.addEventListener("load", loaded); image.addEventListener("error", failed);
              animationCleanups.add(() => { image.removeEventListener("load", loaded); image.removeEventListener("error", failed); image.removeAttribute("src"); });
              image.src = url;
            }
            selected = entry;
            for (const item of images.values()) item.image.hidden = item !== selected;
            update();
          } catch (_) { message.textContent = "Media content is unavailable or invalid."; }
        };
        if (animated) {
          iconAction("Play animation", "play", info, () => show(page.content_index));
          iconAction("Show preview", "image", info, () => show(page.preview_content_index));
        }
        show(animated ? page.preview_content_index : page.content_index);
        if (downloadFiles) downloadControl(linked ? original : {type:"resource",resource:{uri:"urn:pixiv:media:" + output.illust_id + "/" + filename,mimeType:page.mime_type,blob:original.data}}, filename, info, epoch, notifySize);
        else {
          if (downloadURL) {
            downloadControl({type:"resource_link",uri:downloadURL,name:filename,mimeType:page.mime_type,size:page.size}, filename, info, epoch, notifySize);
          } else text("p", "File download is unavailable in this host.", info);
        }
      } catch (_) { message.textContent = "Media content is unavailable or invalid."; }
    }
  }
  function render(result) {
    clearView();
    const epoch = view;
    if (Number.isSafeInteger(result.structuredContent?.illust_id) && result.structuredContent.illust_id > 0 && Array.isArray(result.structuredContent.pages)) { renderMedia(result, epoch); return; }
    const records = result.structuredContent?.records;
    if (!Array.isArray(records)) { status.textContent = "No structured records; use the tool result in chat."; return; }
    status.textContent = result.isError ? "Tool failed or returned partial results." : records.length ? "" : "No results.";
    if (typeof IntersectionObserver === "function") {
      observer = new IntersectionObserver(entries => {
        for (const entry of entries) if (entry.isIntersecting) {
          observer.unobserve(entry.target);
          entry.target.loadPreview();
        }
      });
    }
    let thumbnails;
    // Historical URLs expire or lose their signing key on restart; acquire current capabilities once per view.
    const currentThumbnails = serverTools ? () => thumbnails ??= request("tools/call", {name:"pixiv_show_gallery", arguments:{illust_ids:records.filter(record => record && ["illust", "manga", "ugoira"].includes(record.type) && Number.isSafeInteger(Number(record.id)) && Number(record.id) > 0).map(record => Number(record.id)),page:1,quality:"thumbnail"}}, epoch) : result;
    for (const record of records) {
      const card = document.createElement("article"); card.className = "artwork-card";
      cards.append(card);
      if (!record || typeof record !== "object" || Array.isArray(record)) { text("p", "Invalid record — inspect the tool result in chat.", card, "error"); continue; }
      if (["illust", "manga", "ugoira"].includes(record.type)) {
        const media = proxyImage(record, card, currentThumbnails, epoch, 1, "thumbnail", () => openDetail(record, result));
        card.loadPreview = media.load;
        if (observer) observer.observe(card); else media.load();
      }
    }

  }
  function dispose() {
    if (disposed) return;
    clearView(); clearCache(); disposed = true;
    resize?.disconnect();
    for (const call of pending.values()) call.reject(new Error("Gallery closed"));
    pending.clear();
  }
  window.addEventListener("message", event => {
    if (event.source !== window.parent || disposed) return;
    const message = event.data;
    if (!message || message.jsonrpc !== "2.0") return;
    if (hostOrigin !== "*" && event.origin !== hostOrigin) return;
    if (!message.method && pending.has(message.id)) {
      const call = pending.get(message.id);
      pending.delete(message.id);
      if (message.error) call.reject(new Error("Host request failed"));
      else if (Object.hasOwn(message, "result")) {
        if (hostOrigin === "*" && event.origin && event.origin !== "null") hostOrigin = event.origin;
        call.resolve(message.result);
      } else call.reject(new Error("Invalid host response"));
      return;
    }
    if (!ready) return;
    switch (message.method) {
      case "ui/notifications/host-context-changed": applyContext(message.params); break;
      case "ui/notifications/tool-input":
        lastToolResult = undefined;
        clearView(); clearCache(); status.textContent = "Loading results…"; break;
      case "ui/notifications/tool-result": {
        const result = message.params ?? {};
        const signature = JSON.stringify(result);
        if (signature === lastToolResult) break;
        lastToolResult = signature;
        clearView(); clearCache(); render(result); break;
      }
      case "ui/notifications/tool-cancelled": lastToolResult = undefined; clearView(); clearCache(); status.textContent = "Tool execution cancelled."; break;
      case "ui/resource-teardown": dispose(); send({id: message.id, result: {}}); break;
      case "ping": send({id: message.id, result: {}}); break;
      default: if (message.id !== undefined) send({id: message.id, error: {code: -32601, message: "Method not supported"}});
    }
  });
  window.addEventListener("pagehide", dispose);
  request("ui/initialize", {appInfo: {name: "pixiv-gallery", version: "1"}, appCapabilities: {availableDisplayModes: ["inline"]}, protocolVersion: "2026-01-26"})
    .then(result => {
      if (disposed) return;
      if (result.protocolVersion !== "2026-01-26") throw new Error("Unsupported bridge protocol");
      serverTools = Boolean(result.hostCapabilities?.serverTools);
      downloadFiles = Boolean(result.hostCapabilities?.downloadFile);
      applyContext(result.hostContext);
      ready = true;
      send({method: "ui/notifications/initialized", params: {}});
      status.textContent = "Waiting for tool results…";
      if (typeof ResizeObserver === "function") {
        resize = new ResizeObserver(notifySize);
        resize.observe(document.body);
        window.addEventListener("pagehide", () => resize.disconnect(), {once: true});
      }
    }).catch(() => { if (!disposed) status.textContent = "Gallery bridge unavailable. Use structured results and pixiv_artwork_media in chat."; });
})();
