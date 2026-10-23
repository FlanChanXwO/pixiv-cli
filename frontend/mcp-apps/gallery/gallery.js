
"use strict";
(() => {
  const status = document.getElementById("status");
  const cards = document.getElementById("cards");
  const pending = new Map();
  const urls = new Set();
  const proxyImages = new Set();
  const tagRows = new Set();
  const tagButtons = new Set();
  let tagSearchLocked = false;
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
  let hostOrigin = "*", serverTools = false, downloadFiles = false, chatMessages = false, observer, resize;
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
    const wasFullscreen = hostContext.displayMode === "fullscreen";
    hostContext = {...hostContext, ...update};
    // 宿主已退出全屏，只清理灯箱，不再请求恢复显示模式。
    if (wasFullscreen && hostContext.displayMode !== "fullscreen") closeOriginal?.(false);
    const style = document.documentElement.style;
    style.colorScheme = ["light", "dark"].includes(hostContext.theme) ? hostContext.theme : "light dark";
    const dims = hostContext.containerDimensions ?? {};
    for (const key of ["width", "height", "maxWidth", "maxHeight"]) {
      const value = dimension(dims[key]);
      style[key] = value === undefined ? "" : value + "px";
    }
    style.overflow = "auto";
    // 宿主声明的安全区（刘海、输入栏）优先于浏览器 env()，供全屏灯箱避让。
    const insets = hostContext.safeAreaInsets ?? {};
    for (const side of ["top", "right", "bottom", "left"]) {
      const value = dimension(insets[side]);
      if (value === undefined) style.removeProperty?.("--host-safe-" + side); else style.setProperty?.("--host-safe-" + side, value + "px");
    }
  }
  function notifySize() {
    if (disposed) return;
    for (const row of tagRows) row.update();
    const dims = hostContext.containerDimensions ?? {};
    const availableWidth = dimension(dims.width) ?? Math.min(document.documentElement.scrollWidth, dimension(dims.maxWidth) ?? Infinity);
    const width = lightboxWidth ?? (cards.className === "single-artwork" ? Math.min(dimension(document.body.scrollWidth) ?? availableWidth, availableWidth) : availableWidth);
    const height = Math.min(document.body.scrollHeight, dimension(dims.height) ?? Infinity, dimension(dims.maxHeight) ?? Infinity);
    if (dimension(dims.height) !== undefined) document.documentElement.style.height = height + "px";
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
    tagButtons.clear();
    cards.replaceChildren();
    cards.className = ""; cards.removeAttribute("tabindex");
    status.className = "";
  }
  function text(tag, value, parent, className) {
    const node = document.createElement(tag);
    node.textContent = String(value ?? "");
    if (className) node.className = className;
    parent.append(node);
    return node;
  }
  const iconPaths = {
    close: "m6 6 12 12M6 18 18 6",
    previous: "m15 18-6-6 6-6",
    next: "m9 6 6 6-6 6",
    back: "m15 18-6-6 6-6M9 12h12",
    download: "M12 3v12m-5-5 5 5 5-5M5 16v4h14v-4",
    refresh: "M20 12a8 8 0 1 1-2.34-5.66L20 9M20 4v5h-5",
    heart: "M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z",
    remove: "M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14M10 11v6m4-6v6",
    play: "m9 5 11 7-11 7V5Z",
    image: "M3 3h18v18H3V3Zm0 14 6-6 5 5 3-3 4 4M8 7h.01",
    pages: "M8 3h13v13M3 8h13v13H3V8Z",
    eye: "M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z",
    size: "M3 9V3h6m6 0h6v6M3 15v6h6m6 0h6v-6",
    shrink: "M9 3v6H3m12-6v6h6M9 21v-6H3m12 6v-6h6",
    minus: "M5 12h14",
    plus: "M12 5v14M5 12h14",
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
      author.className = "artwork-author";
      if (validID) { author.title = "Open author page"; author.setAttribute("aria-label", "Open author page — " + record.user.name); }
      return author;
    }
  }
  function pageSize(record, page) {
    const item = Array.isArray(record.pages) ? record.pages.find(item => item?.page_index === page - 1) : undefined;
    const valid = item => Number.isSafeInteger(item?.width) && item.width > 0 && Number.isSafeInteger(item?.height) && item.height > 0;
    return valid(item) ? item : valid(record) ? record : undefined;
  }
  function pageDimensions(record, page) {
    const size = pageSize(record, page);
    return size ? size.width + " × " + size.height : "";
  }
  function artworkMetadata(record, parent) {
    const stats = text("dl", "", parent, "artwork-stats");
    const row = (label, value, name) => {
      const group = document.createElement("div");
      group.className = "metadata-stat"; stats.append(group);
      text("dt", label, group, "sr-only"); group.append(icon(name));
      return text("dd", value, group, "metadata-value");
    };
    const statistics = new Map();
    for (const [key, label, name] of [["total_views", "Views", "eye"], ["total_bookmarks", "Bookmarks", "heart"]]) {
      if (Number.isSafeInteger(record[key]) && record[key] >= 0) statistics.set(key, row(label, String(record[key]), name));
    }
    if (typeof record.id === "string" && /^[1-9][0-9]*$/.test(record.id)) {
      const value = row("Artwork ID", "", "id");
      const copy = action(record.id, value, async () => {
        copy.disabled = true;
        let copied = false;
        try {
          await navigator.clipboard.writeText(record.id);
          copied = true;
        } catch (_) {
          // Some embedded hosts omit clipboard-write permission; use their native selection copy.
          try {
            const range = document.createRange(), selection = document.getSelection();
            range.selectNodeContents(copy); selection.removeAllRanges(); selection.addRange(range);
            copied = document.execCommand("copy");
            if (copied) selection.removeAllRanges();
          } catch (_) {}
        }
        copy.setAttribute("data-copy-state", copied ? "copied" : "failed");
        copy.setAttribute("aria-label", copied ? "Artwork ID copied" : "Copy failed. Artwork ID selected for manual copy.");
        copy.disabled = false;
      });
      copy.className = "pid-copy"; copy.setAttribute("aria-label", "Copy artwork ID");
    }
    const dimensions = pageDimensions(record, 1);
    let size = dimensions ? row("Dimensions", dimensions, "size") : undefined;
    if (typeof record.published_at === "string" && record.published_at !== "0001-01-01T00:00:00Z") {
      const date = new Date(record.published_at);
      if (Number.isFinite(date.getTime())) {
        const value = row("Published", "", "calendar");
        const time = text("time", new Intl.DateTimeFormat("sv-SE", {year:"numeric", month:"2-digit", day:"2-digit"}).format(date), value);
        time.dateTime = record.published_at; time.title = record.published_at;
      }
    }
    return {
      updateDimensions(page) { const value = pageDimensions(record, page); if (!size && value) size = row("Dimensions", value, "size"); if (size) size.textContent = value; },
      updateStats(value) { for (const [key, node] of statistics) node.textContent = Number.isSafeInteger(value?.[key]) && value[key] >= 0 ? String(value[key]) : "—"; }
    };
  }
  function artworkCaption(value, parent, epoch) {
    const decode = value => value.replace(/&(#x[0-9a-f]+|#[0-9]+|amp|lt|gt|quot|apos|nbsp);/gi, (entity, name) => {
      if (!name.startsWith("#")) return {amp:"&", lt:"<", gt:">", quot:'"', apos:"'", nbsp:"\u00a0"}[name.toLowerCase()];
      const code = /^#x/i.test(name) ? Number.parseInt(name.slice(2), 16) : Number(name.slice(1));
      return code > 0 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff) ? String.fromCodePoint(code) : entity;
    });
    const source = String(value).replace(/<br\s*\/?>/gi, "\n");
    if (!/<a\b/i.test(source)) { text("p", decode(source), parent, "caption"); return; }
    const caption = text("p", "", parent, "caption");
    let end = 0;
    // Rebuild supported links from text; never insert upstream HTML or attributes into the document.
    for (const match of source.matchAll(/<a\b([^>]*)>([\s\S]*?)<\/a\s*>/gi)) {
      text("span", decode(source.slice(end, match.index)), caption);
      const label = decode(match[2]);
      const attribute = match[1].match(/(?:^|\s)href\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/i);
      let url;
      try { url = new URL(decode(attribute?.[1] ?? attribute?.[2] ?? attribute?.[3] ?? "")); } catch (_) {}
      if (url && ["https:", "http:"].includes(url.protocol) && !url.username && !url.password) {
        const link = text("a", label, caption, "caption-link");
        link.href = url.href; link.target = "_blank"; link.rel = "noopener noreferrer";
        link.addEventListener("click", event => {
          event.preventDefault();
          request("ui/open-link", {url:url.href}, epoch).catch(() => { if (epoch === view && !disposed) status.textContent = "Link could not be opened."; });
        });
      } else text("span", label, caption);
      end = match.index + match[0].length;
    }
    text("span", decode(source.slice(end)), caption);
  }
  function lockTagSearch(locked) {
    tagSearchLocked = locked;
    for (const chip of tagButtons) {
      chip.disabled = locked;
      chip.title = locked ? "Tag search submitted — continue in chat" : "Search this tag in chat";
    }
  }
  function tagChips(tags, parent, collapsible = false) {
    if (!Array.isArray(tags) || !tags.length) return;
    const group = document.createElement("div"); group.className = "tags"; parent.append(group);
    const list = document.createElement("div"); list.className = "tag-list"; group.append(list);
    for (const tag of tags) {
      if (typeof tag?.name !== "string" || !tag.name) continue;
      if (!chatMessages) { text("span", tag.name, list, "tag"); continue; }
      const epoch = view;
      const chip = action(tag.name, list, async () => {
        if (tagSearchLocked || epoch !== view || disposed) return;
        lockTagSearch(true);
        try {
          // The message outlives local navigation: cancelling a view does not cancel the agent's turn.
          const result = await request("ui/message", {role:"user", content:[{type:"text", text:"Search Pixiv for the exact tag " + JSON.stringify(tag.name) + " and show the matching artworks in a gallery."}]});
          if (result?.isError) throw new Error("Message rejected");
          // shortcut: no host turn-completion signal; keep this instance locked until remount.
        } catch (_) { if (!disposed) { lockTagSearch(false); status.className = ""; status.textContent = "Tag search could not be sent. Try again."; } }
      });
      chip.className = "tag";
      chip.disabled = tagSearchLocked;
      chip.title = tagSearchLocked ? "Tag search submitted — continue in chat" : "Search this tag in chat";
      tagButtons.add(chip);
      chip.setAttribute("aria-label", "Search tag — " + tag.name);
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
  // The tool's entry for one page, if the result covers it at all.
  function capabilityEntry(result, id, page) {
    return result?.structuredContent?.images?.find(item => Number(item?.illust_id) === Number(id) && item.page === page);
  }
  // Only the canonical origin returned by our media tool is allowed; CSP also pins it.
  function capabilityURL(result, id, page) {
    const origin = result?.structuredContent?.media_origin, item = capabilityEntry(result, id, page);
    if (!item || typeof origin !== "string" || !/^https?:\/\/[^/?#@]+$/.test(origin) || typeof item.url !== "string" || !item.url.startsWith(origin + "/") || !item.url.includes("/media/") || /[?#]/.test(item.url)) return undefined;
    return item.url;
  }
  const pageRange = total => Array.from({length: Number.isSafeInteger(total) && total > 0 ? total : 1}, (_, index) => index + 1);
  // Decodes a neighbouring page off-screen so a later turn shows it from the session cache at once.
  function warm(key, url) {
    if (!url || cached(key) || disposed) return;
    const image = document.createElement("img"); image.alt = ""; image.referrerPolicy = "no-referrer";
    image.addEventListener("load", () => { if (!disposed) remember(key, image, image.naturalWidth * image.naturalHeight * 8, () => { if (!proxyImages.has(image)) image.removeAttribute("src"); }); });
    image.src = url;
  }
  // A page turn drawn as a rotating cube over `surface`. The content swaps through `go` once the turn
  // lands, so the existing load, cache and failure paths still decide what is actually shown.
  function cubePager(surface, {current, currentSource, source, valid, go}) {
    let cube, rotor, side = 0, busy = false;
    const motion = () => typeof window.matchMedia === "function" && !window.matchMedia("(prefers-reduced-motion: reduce)").matches && typeof surface.animate === "function" && Boolean(currentSource());
    const half = () => (surface.clientWidth || 0) / 2;
    const place = degrees => { rotor.style.transform = "translateZ(" + -half() + "px) rotateY(" + degrees + "deg)"; };
    function face(src, rotation) {
      const element = document.createElement("div"); element.className = src ? "cube-face" : "cube-face cube-face-pending";
      element.style.transform = "rotateY(" + rotation + "deg) translateZ(" + half() + "px)";
      if (src) { const image = document.createElement("img"); image.alt = ""; image.referrerPolicy = "no-referrer"; image.src = src; element.append(image); }
      rotor.append(element);
    }
    function clear() { cube?.remove(); cube = rotor = undefined; side = 0; surface.classList?.remove("cube-turning"); }
    function build(direction, target) {
      clear();
      cube = document.createElement("div"); cube.className = "cube"; cube.setAttribute("aria-hidden", "true");
      rotor = document.createElement("div"); rotor.className = "cube-rotor"; cube.append(rotor);
      face(currentSource(), 0);
      if (target !== undefined) face(source(target), direction * 90);
      side = direction; surface.append(cube); surface.classList?.add("cube-turning");
    }
    function land(target) {
      const from = rotor.style.transform;
      place(target === undefined ? 0 : -side * 90);
      busy = true;
      const animation = rotor.animate([{transform: from}, {transform: rotor.style.transform}], {duration: 280, easing: "cubic-bezier(.2,.7,.3,1)"});
      animation.onfinish = animation.oncancel = () => {
        busy = false;
        if (target === undefined) { clear(); return; }
        // A decoded target swaps in under the cube; otherwise the normal loading state takes over at once.
        const ready = Boolean(source(target)), pending = go(target);
        if (ready) Promise.resolve(pending).finally(clear); else clear();
      };
    }
    return {
      // Follows a horizontal drag; false means no turn is drawn (reduced motion, busy, nothing shown).
      drag(dx) {
        if (busy || !motion()) return false;
        const direction = dx < 0 ? 1 : -1, target = current() + direction, open = valid(target);
        if (!cube || side !== direction) build(direction, open ? target : undefined);
        // Past the first or last page the cube only gives a little before springing back.
        const limit = open ? 90 : 18;
        place(Math.max(-limit, Math.min(limit, dx / (half() * 2 || 1) * 90)));
        return true;
      },
      release(commit) {
        if (!cube || busy) return false;
        const target = current() + side;
        land(commit && valid(target) ? target : undefined);
        return true;
      },
      // Buttons, keys and the slider turn one face towards the target, even when it is several pages away.
      turn(target) {
        if (!valid(target) || target === current() || busy) return;
        if (!motion()) { go(target); return; }
        const direction = target > current() ? 1 : -1;
        build(direction, target); place(0); land(target);
      }
    };
  }
  function proxyImage(record, parent, initial, epoch, page = 1, quality = "thumbnail", open, openOriginal, onReady, retryAll) {
    const frame = document.createElement("div"); frame.className = "media-frame"; parent.append(frame);
    if (quality !== "thumbnail") mediaBadges(record, frame);
    const key = "image:" + record.id + ":" + page + ":" + quality;
    const retained = cached(key);
    const image = retained?.value ?? document.createElement("img"); image.alt = String(record.title || "Artwork") + " — page " + page; image.hidden = !retained; image.draggable = false; image.referrerPolicy = "no-referrer"; proxyImages.add(image);
    if (open) { const link = action("", frame, open); link.className = "media-open"; link.title = "Open artwork"; link.setAttribute?.("aria-label", "Open " + String(record.title || "artwork")); link.append(image); } else frame.append(image);
    const fitThumbnail = () => { if (quality === "thumbnail" && image.naturalWidth > 0 && image.naturalHeight > 0) frame.style.aspectRatio = image.naturalWidth + " / " + image.naturalHeight; };
    fitThumbnail();
    // Letterboxing shows a blurred copy of the same image rather than flat bars; retained images never fire load.
    const backdrop = () => { if (quality !== "thumbnail" && image.src) frame.style.setProperty?.("--frame-image", "url(" + JSON.stringify(image.src) + ")"); };
    if (retained) backdrop();
    const overlay = document.createElement("div"); overlay.className = "media-state"; frame.append(overlay);
    const message = text("span", "", overlay, "sr-only"); message.setAttribute?.("role", "status");
    const placeholder = text("div", "", overlay, "media-skeleton"); placeholder.setAttribute("aria-hidden", "true");
    const retry = iconAction("Retry image", "refresh", overlay, () => retryAll ? retryAll() : load(true), true); retry.hidden = true; retry.disabled = !serverTools;
    let busy = false, started = Boolean(retained), active = true, broken = false;
    overlay.hidden = Boolean(retained);
    const failed = () => { if (!active || epoch !== view || disposed) return; busy = false; broken = true; forget(key); image.hidden = true; overlay.hidden = false; placeholder.style.display = "none"; message.className = "sr-only"; message.textContent = "Image unavailable"; retry.hidden = false; retry.disabled = !serverTools; };
    const loaded = () => {
      if (!active || epoch !== view || disposed) return;
      busy = false; broken = false; image.hidden = false; overlay.hidden = true; retry.hidden = true; fitThumbnail();
      backdrop();
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
      const url = capabilityURL(result, record.id, page);
      if (!url) throw new Error("Invalid image URL");
      image.src = url;
    };
    const load = async (renew, source) => {
      if (retained && !renew && active) { onReady?.(); return; }
      if (!active || busy || epoch !== view || disposed || (started && !renew)) return;
      started = true; busy = true; broken = false; retry.hidden = true; overlay.hidden = false; placeholder.style.display = ""; message.className = "sr-only"; message.textContent = "Loading image…";
      try {
        if (!renew && typeof initial === "function") { const response = await initial(); if (!active || epoch !== view || disposed) return; assign(response); }
        else if (!renew && initial?.structuredContent?.images?.some(p => Number(p.illust_id) === Number(record.id) && p.page === page)) assign(initial);
        else if (source) { const response = await source(); if (!active || epoch !== view || disposed) return; assign(response); }
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
    return {load: () => load(false), renew: source => load(true, source), failed: () => broken, frame, image, dispose, page};
  }
  function action(label, parent, handler) {
    const button = text("button", label, parent);
    button.type = "button";
    button.addEventListener("click", handler);
    return button;
  }
  function imageURL(content, page) {
    if (!content || content.type !== "image" || typeof content.data !== "string" || !["image/png","image/jpeg","image/webp","image/gif","image/bmp"].includes(content.mimeType)) throw new Error("Invalid image content");
    const decoded = atob(content.data);
    const bytes = new Uint8Array(decoded.length);
    for (let i = 0; i < decoded.length; i++) bytes[i] = decoded.charCodeAt(i);
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
      return;
    }
    const message = text("p", "", parent); message.setAttribute("role", "status");
    let busy = false;
    const button = iconAction("Download " + filename, "download", parent, async () => {
      if (!downloadFiles || busy || epoch !== view || !parent.contains(button)) return;
      busy = true; button.disabled = true; message.textContent = "Waiting for host download confirmation…";
      try {
        const result = await request("ui/download-file", {contents: [content]}, epoch);
        if (epoch !== view || disposed) return;
        if (!result || result.isError) throw new Error("Download not confirmed");
        message.textContent = "Host confirmed download.";
      } catch (_) { if (epoch === view && parent.contains(button)) { message.textContent = "Download not confirmed."; failed(); } }
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
  async function downloadOriginal(id, page, epoch, artwork) {
    const content = await originalContent(id, page, epoch, artwork);
    if (epoch !== view || disposed) return;
    const saved = await request("ui/download-file", {contents:[{type:"resource",resource:{uri:"urn:pixiv:artwork:" + id + ":page:" + page,mimeType:content.mimeType,blob:content.data}}]}, epoch);
    if (!saved || saved.isError) throw new Error("Download rejected");
  }
  let closeOriginal, lightboxWidth;
  function originalViewer(id, page, artwork, epoch) {
    if (closeOriginal || epoch !== view || disposed) return;
    closeOriginal = cleanup;
    id = Number(id);
    const total = Number.isSafeInteger(artwork.page_count) && artwork.page_count > 0 ? artwork.page_count : 1;
    let closed = false, selectedPage = Math.max(1, Math.min(total, page)), requestedPage = selectedPage, token = 0, frontURL = "", pendingURL = "", pendingPage = 0;
    let fitWidth = 800, fitHeight = 600, zoom = "fit", shown = false, refit, placeholder, originalBatch;
    const dialog = document.createElement("dialog"); dialog.className = "original-viewer";
    dialog.setAttribute("aria-label", String(artwork.title || "Artwork")); cards.append(dialog);
    const top = text("div", "", dialog, "original-top");
    // A fullscreen host draws its own close control; ours stays for inline hosts, with Escape as well.
    const close = iconAction("Close original", "close", top, cleanup, true); close.hidden = hostContext.displayMode === "fullscreen";
    // Fullscreen hosts may float their own input over the bottom; CSS keeps the dock above it.
    if (close.hidden) dialog.setAttribute("data-fullscreen", "");
    const stage = text("div", "", dialog, "original-stage");
    let front = document.createElement("img"); front.className = "original-image"; front.alt = ""; front.hidden = true; front.referrerPolicy = "no-referrer"; stage.append(front);
    let back = document.createElement("img"); back.className = "original-image original-incoming"; back.alt = ""; back.hidden = true; back.referrerPolicy = "no-referrer"; stage.append(back);
    const spinner = text("div", "", stage, "original-loading"); spinner.setAttribute("aria-hidden", "true");
    const status = text("p", "", dialog, "sr-only"); status.setAttribute("role", "status");
    const error = text("div", "", dialog, "original-error"); error.hidden = true;
    const errorLabel = text("span", "Image unavailable", error, "sr-only"); error.append(icon("image"));
    const retry = iconAction("Retry image", "refresh", error, () => load(requestedPage), true); retry.className = "icon-button original-retry";
    const previous = iconAction("Previous image", "previous", dialog, () => show(selectedPage - 1), true); previous.className = "icon-button original-side original-prev";
    const next = iconAction("Next image", "next", dialog, () => show(selectedPage + 1), true); next.className = "icon-button original-side original-next";
    if (total === 1) previous.hidden = next.hidden = true;
    const dock = text("div", "", dialog, "original-dock");
    const counter = text("span", "", dock, "original-count"); counter.hidden = total === 1;
    if (total > 1) counter.append(icon("pages"));
    const pageLabel = text("span", "", counter);
    // Stepped zoom is for mouse users; touch screens pinch instead (hidden for coarse pointers).
    const out = iconAction("Zoom out", "minus", dock, () => scale(-0.25), true); out.className = "icon-button original-step";
    const readout = text("span", "Fit", dock, "original-zoom");
    const plus = iconAction("Zoom in", "plus", dock, () => scale(0.25), true); plus.className = "icon-button original-step";
    text("span", "", dock, "original-divider");
    const fit = iconAction("Actual size", "size", dock, toggle, true); fit.setAttribute("aria-pressed", "false");
    let downloadButton;
    if (downloadFiles) {
      downloadButton = iconAction("Download viewed original", "download", dock, download, true);
      downloadButton.disabled = true;
    } else {
      // 复用当前页经验证的原图地址，避免为浏览器下载再请求一次。
      downloadButton = text("a", "", dock, "icon-button original-download");
      downloadButton.setAttribute("aria-label", "Download viewed original"); downloadButton.title = "Download viewed original";
      downloadButton.append(icon("download")); downloadButton.hidden = true;
    }
    // Controls only appear once an image can be acted on.
    const toggleControls = enabled => { out.disabled = plus.disabled = fit.disabled = !enabled; dock.hidden = !enabled; };
    toggleControls(false);
    function setLoading(loading) { spinner.hidden = !loading; dialog.setAttribute("aria-busy", String(loading)); }
    // A fullscreen host may size its panel to the image instead of a fixed wide pane.
    function preferImageWidth() {
      if (!shown || hostContext.displayMode !== "fullscreen" || !(front.naturalWidth > 0 && front.naturalHeight > 0)) return;
      const {x, y} = stagePadding(), height = (Number(stage.clientHeight) || 0) - y;
      if (height > 0) { lightboxWidth = Math.round(Math.min(front.naturalWidth, height * front.naturalWidth / front.naturalHeight) + x); notifySize(); }
    }
    function stagePadding() {
      const computed = globalThis.getComputedStyle?.(stage);
      // A real 0px padding must stay 0; only an unavailable style falls back to 16px.
      const pad = name => { const value = parseFloat(computed?.[name]); return Number.isFinite(value) ? value : 16; };
      return {x: pad("paddingLeft") + pad("paddingRight"), y: pad("paddingTop") + pad("paddingBottom")};
    }
    function fitFrom(image) {
      const box = stage.getBoundingClientRect?.() ?? {width: stage.clientWidth || 800, height: stage.clientHeight || 600};
      const {x: padX, y: padY} = stagePadding();
      const width = Math.max(1, (Number(box.width) || 0) - padX), height = Math.max(1, (Number(box.height) || 0) - padY);
      const ratio = (Number(image.naturalWidth) || 1) / (Number(image.naturalHeight) || 1) || 1;
      if (width / height > ratio) { fitHeight = height; fitWidth = Math.max(1, Math.round(height * ratio)); }
      else { fitWidth = width; fitHeight = Math.max(1, Math.round(width / ratio)); }
    }
    function applyZoom(next) {
      if (!shown) return;
      zoom = next;
      const actual = zoom === "actual", fitted = zoom === "fit", percent = actual || fitted ? 1 : Math.max(0.5, Math.min(4, zoom));
      // Fit is pure CSS so any host resize (panel, maximize, rotation) re-fits without measuring.
      front.className = "original-image" + (actual ? " original-actual" : fitted ? " original-fit" : "");
      if (fitted) front.style.width = front.style.height = "";
      else {
        if (!actual) fitFrom(front);
        front.style.width = (actual ? front.naturalWidth : Math.round(fitWidth * percent)) + "px";
        front.style.height = (actual ? front.naturalHeight : Math.round(fitHeight * percent)) + "px";
      }
      readout.textContent = actual ? "1:1" : percent === 1 ? "Fit" : Math.round(percent * 100) + "%";
      fit.setAttribute("aria-pressed", String(actual)); fit.setAttribute("aria-label", actual ? "Fit to window" : "Actual size");
      // The icon shows what a tap will do, since the pressed state has no background on touch.
      fit.replaceChildren(icon(actual ? "shrink" : "size"));
      stage.className = "original-stage" + (actual || percent > 1 ? " original-zoomed" : "");
    }
    function scale(delta) {
      if (!shown) return;
      const percent = zoom === "actual" || zoom === "fit" ? 1 : zoom;
      applyZoom(Math.max(0.5, Math.min(4, Math.round((percent + delta) * 100) / 100)));
    }
    function toggle() { if (shown) applyZoom(zoom === "actual" ? 1 : "actual"); }
    function goTo(number) {
      if (!Number.isSafeInteger(number) || number < 1 || number > total) return;
      if (number === selectedPage && shown) return;
      return load(number);
    }
    const pager = cubePager(stage, {current: () => selectedPage, currentSource: () => shown && !front.hidden ? frontURL : "", source: number => cached("original-image:" + id + ":" + number)?.value?.src || cached("image:" + id + ":" + number + ":regular")?.value?.src || "", valid: number => Number.isSafeInteger(number) && number >= 1 && number <= total, go: goTo});
    const show = number => pager.turn(number);
    function dropPlaceholder() { placeholder?.remove(); placeholder = undefined; }
    function failed() {
      if (closed) return;
      // Expired or rejected capabilities must not be reused by the explicit retry.
      originalBatch = undefined;
      // 渐进加载失败时撤下半张原图，恢复此前已显示的页面。
      dropPlaceholder(); back.hidden = true; if (shown) front.hidden = false;
      setLoading(false); error.hidden = false; retry.hidden = false; status.textContent = "Image unavailable";
      pageLabel.textContent = total > 1 ? selectedPage + " / " + total : "";
      toggleControls(false); if (downloadFiles) downloadButton.disabled = true; else downloadButton.hidden = true;
    }
    function releaseImage(image) {
      image.remove(); proxyImages.delete(image);
      if (sessionCache.get("original-image:" + id + ":" + image.getAttribute("data-original-page"))?.value !== image) image.removeAttribute("src");
    }
    async function download() {
      if (!downloadFiles || !shown || downloadButton.disabled) return;
      downloadButton.disabled = true;
      try { await downloadOriginal(id, selectedPage, epoch, artwork); if (!closed) status.textContent = "Download requested."; }
      catch (_) { if (!closed) { retry.hidden = true; errorLabel.textContent = "Download failed"; error.hidden = false; status.textContent = "Download failed."; } }
      finally { if (!closed) downloadButton.disabled = false; }
    }
    function commit(progressive) {
      if (closed || !pendingURL) return;
      if (!back.naturalWidth) return;
      dropPlaceholder();
      const old = front; front = back; back = old;
      back.hidden = true; back.className = "original-image original-incoming";
      front.className = "original-image"; front.hidden = false; front.style.visibility = ""; frontURL = pendingURL; pendingURL = ""; selectedPage = pendingPage;
      requestedPage = selectedPage;
      shown = true; applyZoom("fit"); preferImageWidth();
      if (!progressive && !window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) front.animate?.([{opacity:0},{opacity:1}], {duration:180, easing:"ease-out"});
      back.style.visibility = "";
      setLoading(false); error.hidden = true; retry.hidden = true; toggleControls(true);
      previous.disabled = previous.hidden = total === 1 || selectedPage === 1; next.disabled = next.hidden = total === 1 || selectedPage === total;
      pageLabel.textContent = total > 1 ? selectedPage + " / " + total : "";
      if (downloadFiles) downloadButton.disabled = false;
      else { downloadButton.href = frontURL + "?download=1"; downloadButton.target = "_blank"; downloadButton.rel = "noreferrer"; downloadButton.hidden = false; }
      status.textContent = "Image ready.";
    }
    async function load(number) {
      if (closed || epoch !== view || disposed || !Number.isSafeInteger(number) || number < 1 || number > total) return;
      const mine = ++token; requestedPage = number;
      setLoading(true); error.hidden = true; retry.hidden = true; previous.disabled = !shown;
      // 页码跟随目标页，加载期间就能看出要切到哪一张。
      pageLabel.textContent = total > 1 ? number + " / " + total : "";
      toggleControls(shown); if (downloadFiles) downloadButton.disabled = true; else downloadButton.hidden = true;
      status.textContent = "Loading image…";
      try {
        const key = "original-image:" + id + ":" + number;
        const retained = cached(key)?.value;
        let nextURL = retained?.src;
        if (!retained) {
          // One call signs every original page; a page the batch does not cover falls back to its own call.
          originalBatch ??= request("tools/call", {name:"pixiv_show_gallery",arguments:{illust_ids:[id],pages:pageRange(total),quality:"original"}}, epoch);
          let result = await originalBatch;
          if (closed || mine !== token || epoch !== view || disposed) return;
          if (!capabilityEntry(result, id, number)) {
            result = await request("tools/call", {name:"pixiv_show_gallery",arguments:{illust_ids:[id],page:number,quality:"original"}}, epoch);
            if (closed || mine !== token || epoch !== view || disposed) return;
          }
          nextURL = capabilityURL(result, id, number);
          if (!nextURL) throw new Error("Invalid original delivery");
        }
        if (closed || mine !== token || epoch !== view || disposed) return;
        releaseImage(back); dropPlaceholder();
        // 详情页已加载的同页 regular 图垫底，原图边下载边绘制覆盖上去。
        const regular = !retained && cached("image:" + id + ":" + number + ":regular")?.value?.src;
        const progressive = !retained && (Boolean(regular) || front.hidden);
        if (regular) {
          placeholder = document.createElement("img"); placeholder.className = "original-image original-placeholder"; placeholder.alt = ""; placeholder.referrerPolicy = "no-referrer";
          stage.append(placeholder); placeholder.src = regular;
        }
        const incoming = retained ?? document.createElement("img"); back = incoming; stage.append(incoming); proxyImages.add(incoming);
        incoming.alt = ""; incoming.referrerPolicy = "no-referrer"; incoming.setAttribute("data-original-page", number);
        pendingURL = nextURL; pendingPage = number;
        dialog.setAttribute("data-progressive", String(progressive));
        incoming.hidden = false;
        if (progressive) { front.hidden = true; incoming.className = "original-image original-streaming"; incoming.style.visibility = ""; }
        // Without a placeholder, keep the decoded page visible until the original HTTP stream has decoded.
        else { incoming.className = "original-image original-incoming"; incoming.style.visibility = "hidden"; }
        if (!retained) incoming.src = nextURL;
        try { await incoming.decode(); } catch (_) { if (mine === token && !closed) { forget(key); failed(); } return; }
        if (closed || mine !== token || epoch !== view || disposed) return;
        commit(progressive);
        remember(key, front, front.naturalWidth * front.naturalHeight * 8, () => { if (!proxyImages.has(incoming)) incoming.removeAttribute("src"); });
        // Neighbouring originals decode in the background from the same batch, so the next turn is instant.
        for (const near of [number + 1, number - 1]) {
          if (near >= 1 && near <= total) originalBatch?.then(batch => { if (!closed) warm("original-image:" + id + ":" + near, capabilityURL(batch, id, near)); }, () => {});
        }
      } catch (_) { if (mine === token && !closed && epoch === view && !disposed) failed(); }
    }
    stage.addEventListener("click", event => {
      if (event.target !== front) return;
      if (moved) { moved = false; return; }
      toggle();
    });
    stage.addEventListener("wheel", event => { if (shown) { event.preventDefault(); scale(event.deltaY < 0 ? 0.25 : -0.25); } }, {passive:false});
    let drag, moved = false;
    stage.addEventListener("pointerdown", event => {
      if (!shown || event.button !== 0 || event.target !== front) return;
      drag = {id:event.pointerId, x:event.clientX, y:event.clientY, left:stage.scrollLeft || 0, top:stage.scrollTop || 0}; moved = false;
      stage.setPointerCapture?.(event.pointerId);
    });
    stage.addEventListener("pointermove", event => {
      if (!drag || drag.id !== event.pointerId) return;
      const dx = event.clientX - drag.x, dy = event.clientY - drag.y;
      if (Math.abs(dx) + Math.abs(dy) > 8) moved = true;
      if (zoom === "actual" || zoom > 1) { stage.scrollLeft = drag.left - dx; stage.scrollTop = drag.top - dy; }
    });
    const endDrag = () => { drag = undefined; };
    stage.addEventListener("pointerup", endDrag);
    stage.addEventListener("pointercancel", endDrag);
    let pinch, swipe;
    stage.addEventListener("touchstart", event => {
      if (event.touches?.length === 2) {
        const [a,b] = event.touches; pinch = {distance:Math.hypot(a.clientX-b.clientX,a.clientY-b.clientY), zoom:typeof zoom === "number" ? zoom : 1}; // fit and actual both start from the fitted scale
      } else if (event.touches?.length === 1 && (zoom === "fit" || zoom === 1)) swipe = {x:event.touches[0].clientX, y:event.touches[0].clientY};
    }, {passive:true});
    stage.addEventListener("touchmove", event => {
      if (swipe && event.touches?.length === 1) {
        const dx = event.touches[0].clientX - swipe.x, dy = event.touches[0].clientY - swipe.y;
        if (!swipe.horizontal && Math.abs(dx) > 8 && Math.abs(dx) > Math.abs(dy) * 1.5) swipe.horizontal = true;
        if (swipe.horizontal && pager.drag(dx)) event.preventDefault();
        return;
      }
      if (!pinch || event.touches?.length !== 2) return;
      const [a,b] = event.touches, distance = Math.hypot(a.clientX-b.clientX,a.clientY-b.clientY);
      if (pinch.distance > 0) applyZoom(Math.max(0.5, Math.min(4, Math.round(pinch.zoom * distance / pinch.distance * 100) / 100)));
      event.preventDefault();
    }, {passive:false});
    stage.addEventListener("touchend", event => {
      if (pinch) { pinch = undefined; return; }
      if (!swipe || (event.changedTouches?.length ?? 0) !== 1) return;
      const touch = event.changedTouches[0], dx = touch.clientX - swipe.x, dy = touch.clientY - swipe.y; swipe = undefined;
      const commit = Math.abs(dx) >= 44 && Math.abs(dx) > Math.abs(dy) * 1.5;
      if (!pager.release(commit) && commit) goTo(selectedPage + (dx < 0 ? 1 : -1));
    }, {passive:true});
    dialog.addEventListener("keydown", event => {
      if (event.key === "ArrowLeft") { event.preventDefault(); show(selectedPage - 1); }
      else if (event.key === "ArrowRight") { event.preventDefault(); show(selectedPage + 1); }
      else if (event.key === "+" || event.key === "=") { event.preventDefault(); scale(0.25); }
      else if (event.key === "-" || event.key === "_") { event.preventDefault(); scale(-0.25); }
      else if (event.key === "0") { event.preventDefault(); applyZoom("fit"); }
    });
    dialog.addEventListener("cancel", event => { event.preventDefault(); cleanup(); });
    animationCleanups.add(cleanup); dialog.showModal(); load(selectedPage);
    // 宿主若仍按详情页高度渲染 iframe，fixed 灯箱会被滚出可视区；进入 fullscreen 才能贴住真实窗口。
    let restoreDisplay;
    if ((hostContext.availableDisplayModes ?? []).includes("fullscreen") && hostContext.displayMode !== "fullscreen") {
      const previousDisplay = hostContext.displayMode ?? "inline";
      request("ui/request-display-mode", {mode: "fullscreen"}).then(result => {
        if (result?.mode !== "fullscreen") return;
        if (closed) { request("ui/request-display-mode", {mode: previousDisplay}).catch(() => {}); return; }
        close.hidden = true; dialog.setAttribute("data-fullscreen", ""); preferImageWidth();
        restoreDisplay = () => request("ui/request-display-mode", {mode: previousDisplay}).catch(() => {});
      }).catch(() => {});
    }
    // 容器尺寸变化（例如切换 fullscreen）后重新按 Fit 计算，避免图片停在旧尺寸。
    if (typeof ResizeObserver === "function") {
      refit = new ResizeObserver(() => { if (shown && zoom !== "fit") applyZoom(zoom); });
      refit.observe(stage);
    }
    function cleanup(restore = true) {
      if (closed) return; closed = true; closeOriginal = undefined;
      if (lightboxWidth !== undefined) { lightboxWidth = undefined; notifySize(); }
      refit?.disconnect(); if (restore) restoreDisplay?.();
      for (const image of [front, back]) releaseImage(image);
      dialog.close(); dialog.remove(); animationCleanups.delete(cleanup);
    }
  }
  async function openDetail(record, previous) {
    clearView();
    const epoch = view;
    const id = Number(record.id);
    if (previous) iconAction("Back to results", "back", cards, () => render(previous), true);
    if (!serverTools || !Number.isSafeInteger(id) || id <= 0) { status.textContent = "Artwork detail unavailable."; return; }
    status.textContent = "Loading artwork…"; status.className = "sr-only";
    const skeleton = text("div", "", cards, "detail-skeleton"); skeleton.setAttribute("aria-hidden", "true");
    text("div", "", skeleton, "skeleton-image"); text("div", "", skeleton, "skeleton-copy");
    try {
      const metadataKey = "detail:" + id;
      const result = cached(metadataKey)?.value ?? await request("tools/call", {name: "pixiv_illust_detail", arguments: {illust_id: id}}, epoch);
      if (epoch !== view || disposed) return;
      skeleton.remove();
      const artwork = result.structuredContent?.records?.find(item => item && Number(item.id) === id);
      if (result.isError || !artwork) throw new Error("Artwork unavailable");
      remember(metadataKey, result, JSON.stringify(result).length * 2);
      status.textContent = ""; status.className = "";
      const section = document.createElement("article"); section.className = "artwork-detail"; cards.append(section);
      const mediaColumn = document.createElement("div"); mediaColumn.className = "detail-media"; section.append(mediaColumn);
      const sheet = text("div", "", section, "detail-info");
      const toggle = action("", sheet, () => { if (dragged) { dragged = false; return; } setCollapsed(!info.hidden); });
      toggle.className = "detail-toggle";
      const grip = text("span", "", toggle, "detail-grip"); grip.setAttribute("aria-hidden", "true");
      const info = text("div", "", sheet, "detail-content"); info.id = "artwork-detail-content";
      toggle.setAttribute("aria-controls", info.id);
      let start, dragged = false;
      const setCollapsed = collapsed => {
        info.style.height = ""; info.style.overflow = "";
        info.hidden = collapsed; toggle.setAttribute("aria-expanded", String(!collapsed));
        toggle.setAttribute("aria-label", collapsed ? "Show details" : "Hide details");
        notifySize();
      };
      setCollapsed(false);
      toggle.addEventListener("pointerdown", event => {
        if (event.button !== 0 || event.isPrimary === false) return;
        start = {id:event.pointerId, y:event.clientY, collapsed:info.hidden};
        info.hidden = false; start.height = info.scrollHeight;
        info.style.height = (start.collapsed ? 0 : start.height) + "px"; info.style.overflow = "hidden";
        dragged = false; toggle.setPointerCapture?.(event.pointerId);
      });
      toggle.addEventListener("pointermove", event => {
        if (!start || start.id !== event.pointerId) return;
        const dy = event.clientY - start.y;
        if (Math.abs(dy) > 8) dragged = true;
        info.style.height = Math.max(0, Math.min(start.height, (start.collapsed ? 0 : start.height) - dy)) + "px";
      });
      toggle.addEventListener("pointerup", event => {
        if (!start || start.id !== event.pointerId) return;
        const dy = event.clientY - start.y, collapsed = start.collapsed; start = undefined;
        if (Math.abs(dy) >= 32) { dragged = true; setCollapsed(dy > 0); }
        else setCollapsed(collapsed);
      });
      toggle.addEventListener("pointercancel", () => { if (start) setCollapsed(start.collapsed); start = undefined; dragged = false; });
      toggle.addEventListener("keydown", event => { if (event.key === "Enter" || event.key === " ") dragged = false; });
      text("h2", artwork.title, info);
      const authorRow = text("div", "", info, "detail-author-row");
      const authorCard = text("div", "", authorRow, "author-card");
      const author = artworkAuthor(artwork, authorCard);
      let initialPreview;
      // One call signs every page; a rejected batch is dropped so a later retry asks again.
      const detailPreview = () => initialPreview ??= request("tools/call", {name:"pixiv_show_gallery", arguments:{illust_ids:[id], pages:pageRange(artwork.page_count), quality:"regular"}}, epoch).catch(error => { initialPreview = undefined; throw error; });
      // A page the batch does not cover falls back to its own call; a covered but invalid entry still fails.
      const pageSource = page => async () => {
        const batch = await detailPreview();
        return capabilityEntry(batch, id, page) ? batch : request("tools/call", {name:"pixiv_show_gallery", arguments:{illust_ids:[id], page, quality:"regular"}}, epoch);
      };
      if (author && artwork.user?.profile_image?.resource) {
        const avatarKey = "avatar:" + id;
        const avatarURL = cached(avatarKey)?.value;
        const avatar = document.createElement("img"); avatar.className = "author-avatar"; avatar.alt = ""; avatar.hidden = true; avatar.referrerPolicy = "no-referrer"; author.append(avatar); proxyImages.add(avatar); author.setAttribute("data-avatar-loading", "true");
        const message = text("span", "", authorCard, "sr-only"); message.setAttribute("role", "status");
        const failed = () => { if (epoch === view && !disposed) { forget(avatarKey); author.removeAttribute("data-avatar-loading"); avatar.hidden = true; message.textContent = "Avatar unavailable"; } };
        avatar.addEventListener("load", () => { if (epoch === view && !disposed) { author.removeAttribute("data-avatar-loading"); avatar.hidden = false; message.textContent = ""; remember(avatarKey, avatar.src, avatar.src.length * 2); } });
        avatar.addEventListener("error", failed);
        if (avatarURL) avatar.src = avatarURL;
        else detailPreview().then(result => {
          if (epoch !== view || disposed) return;
          const output = result.structuredContent;
          const url = output?.images?.find(item => Number(item.illust_id) === id)?.avatar_url;
          if (typeof output?.media_origin !== "string" || !/^https?:\/\/[^/?#@]+$/.test(output.media_origin) || typeof url !== "string" || !url.startsWith(output.media_origin + "/media/") || /[?#]/.test(url)) { failed(); return; }
          avatar.src = url;
        }).catch(failed);
      }
      if (artwork.type === "ugoira") ratingBadge(artwork, info);
      tagChips(artwork.tags, info, true);
      if (artwork.caption) artworkCaption(artwork.caption, info, epoch);
      const {updateDimensions, updateStats} = artworkMetadata(artwork, info);
      const detailControls = document.createElement("div"); detailControls.className = "detail-controls";
      if (artwork.type === "ugoira") {
        animationControls(id, artwork, mediaColumn, epoch);
      } else {
        const total = artwork.page_count;
        if (!Number.isSafeInteger(total) || total < 1) throw new Error("Invalid page count");
        const viewer = document.createElement("div"); viewer.className = "page-viewer"; mediaColumn.append(viewer);
        const media = document.createElement("div"); media.className = "page-media"; viewer.append(media);
        let selectedPage = 1, preview, displayed, previousPage, nextPage, progress, position, counter, counterLabel;
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
            const replacement = proxyImage(artwork, media, pageSource(page), epoch, page, "regular", undefined, () => originalViewer(id, page, artwork, epoch), () => {
              if (selectedPage !== page || epoch !== view || disposed) return;
              if (displayed) { displayed.dispose(); displayed.frame.remove(); }
              displayed = replacement; replacement.frame.className = "media-frame";
              // Warm neighbours only from a batch already requested; speculation never costs a host round trip.
              for (const near of [page + 1, page - 1]) {
                if (validPage(near)) initialPreview?.then(batch => { if (epoch === view) warm("image:" + id + ":" + near + ":regular", capabilityURL(batch, id, near)); }, () => {});
              }
            });
            preview = replacement;
            // Size the frame from the artwork, not vh: the host resizes this iframe to its content height.
            const size = pageSize(artwork, page);
            if (size) replacement.frame.style.aspectRatio = size.width + " / " + size.height;
            if (displayed) replacement.frame.className = "media-frame page-pending";
            replacement.load();
          }
          if (progress) {
            progress.value = String(page); position.textContent = counterLabel.textContent = page + " / " + total;
            // Unavailable directions disappear instead of showing a dead control.
            previousPage.disabled = previousPage.hidden = page === 1; nextPage.disabled = nextPage.hidden = page === total;
          }
        };
        if (total > 1) {
          const pager = cubePager(media, {current: () => selectedPage, currentSource: () => displayed?.image.src || "", source: page => cached("image:" + id + ":" + page + ":regular")?.value?.src || "", valid: validPage, go: loadPage});
          previousPage = iconAction("Previous image", "previous", viewer, () => pager.turn(selectedPage - 1), true); previousPage.className = "icon-button page-previous";
          nextPage = iconAction("Next image", "next", viewer, () => pager.turn(selectedPage + 1), true); nextPage.className = "icon-button page-next";
          // The slider is on demand: the counter pill opens it as an overlay instead of reserving a row.
          const navigation = document.createElement("div"); navigation.className = "page-navigation"; navigation.hidden = true; viewer.append(navigation);
          counter = action("", viewer, () => {
            navigation.hidden = !navigation.hidden;
            counter.setAttribute("aria-expanded", String(!navigation.hidden));
            counter.setAttribute("aria-label", navigation.hidden ? "Show page slider" : "Hide page slider");
          });
          counter.className = "page-counter"; counter.setAttribute("aria-expanded", "false"); counter.setAttribute("aria-label", "Show page slider");
          // The button keeps a full touch target; only its label is drawn as the small pill.
          counterLabel = text("span", "", counter, "page-counter-label");
          progress = document.createElement("input"); progress.type = "range"; progress.min = "1"; progress.max = String(total); progress.step = "1"; progress.setAttribute("aria-label", "Image progress"); navigation.append(progress);
          position = text("output", "", navigation, "page-position");
          progress.addEventListener("input", () => { const page = Number(progress.value); if (validPage(page)) position.textContent = counterLabel.textContent = page + " / " + total; });
          progress.addEventListener("change", () => { const page = Number(progress.value); if (validPage(page) && page !== selectedPage) pager.turn(page); });
          let gesture;
          media.addEventListener("pointerdown", event => { if (event.target?.closest("button")) return; if (event.isPrimary !== false && event.button === 0) { swiped = false; gesture = {id:event.pointerId, x:event.clientX, y:event.clientY, horizontal:false}; media.setPointerCapture?.(event.pointerId); } });
          // Once a drag is clearly horizontal the cube follows the finger; pan-y keeps vertical scrolling with the chat.
          media.addEventListener("pointermove", event => {
            if (!gesture || gesture.id !== event.pointerId) return;
            const dx = event.clientX - gesture.x, dy = event.clientY - gesture.y;
            if (!gesture.horizontal && Math.abs(dx) > 8 && Math.abs(dx) > Math.abs(dy) * 1.5) gesture.horizontal = true;
            if (gesture.horizontal) { swiped = true; pager.drag(dx); }
          });
          media.addEventListener("pointercancel", () => { gesture = undefined; pager.release(false); });
          media.addEventListener("pointerup", event => {
            const start = gesture; gesture = undefined;
            if (!start || start.id !== event.pointerId) return;
            const dx = event.clientX - start.x, dy = event.clientY - start.y;
            // A full touch target of horizontal travel distinguishes swipes from taps.
            const commit = Math.abs(dx) >= 44 && Math.abs(dx) > Math.abs(dy) * 1.5;
            if (!start.horizontal && !commit) return;
            swiped = true; event.preventDefault();
            if (!pager.release(commit) && commit) { const page = selectedPage + (dx < 0 ? 1 : -1); if (validPage(page)) loadPage(page); }
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
      bookmarkControls(id, authorRow, epoch, async () => {
        forget(metadataKey); updateStats(undefined);
        const refreshed = await request("tools/call", {name:"pixiv_illust_detail", arguments:{illust_id:id}}, epoch);
        if (epoch !== view || disposed) return;
        const value = refreshed.structuredContent?.records?.find(item => Number(item?.id) === id);
        if (refreshed.isError || !value) throw new Error("Statistics unavailable");
        remember(metadataKey, refreshed, JSON.stringify(refreshed).length * 2); updateStats(value);
      });
    } catch (_) {
      if (epoch === view && !disposed) {
        skeleton.remove(); status.className = "sr-only"; status.textContent = "Artwork detail failed. Retry or return to results.";
        const failure = text("div", "", cards, "detail-failure");
        iconAction("Retry artwork detail", "refresh", failure, () => openDetail(record, previous), true);
      }
    }
  }
  function bookmarkControls(id, section, epoch, refreshStatistics) {
    const group = document.createElement("div"); group.className = "bookmark-controls"; section.append(group);
    const message = text("p", "Loading bookmark state…", group, "sr-only"); message.setAttribute("role", "status");
    const toolbar = document.createElement("div"); toolbar.className = "bookmark-toolbar"; group.append(toolbar);
    const options = text("div", "", group, "bookmark-options"); options.hidden = true;
    options.setAttribute("role", "dialog"); options.setAttribute("aria-label", "Bookmark settings");
    const heading = text("div", "", options, "bookmark-options-heading");
    text("strong", "Bookmark settings", heading);
    let state, busy = false, hold, gesture, held = false;
    const cancelHold = () => { clearTimeout(hold); hold = undefined; gesture = undefined; };
    const openOptions = () => {
      if (busy || !state || epoch !== view || disposed) return;
      options.hidden = false; save.setAttribute("aria-expanded", "true"); visibility.focus?.();
    };
    const closeOptions = () => { options.hidden = true; save.setAttribute("aria-expanded", "false"); };
    const outsidePress = event => { if (!options.hidden && !group.contains(event.target)) closeOptions(); };
    window.addEventListener("pointerdown", outsidePress);
    animationCleanups.add(() => window.removeEventListener?.("pointerdown", outsidePress));
    iconAction("Close settings", "close", heading, () => { closeOptions(); save.focus?.(); }, true);
    const enable = () => {
      save.disabled = apply.disabled = remove.disabled = visibility.disabled = busy || !state;
      refresh.disabled = busy;
      // Unknown state swaps the heart for an icon-only retry; the reason stays in the assistive status.
      save.hidden = !state && !busy;
      retry.disabled = busy; retry.hidden = Boolean(state) || busy;
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
        const label = value.bookmarked ? "Unbookmark" : "Bookmark";
        save.textContent = value.bookmarked ? "Bookmarked" : "Bookmark"; save.append(icon("heart")); save.setAttribute("aria-label", label); save.title = label + " · hold for settings";
        save.setAttribute("aria-pressed", String(value.bookmarked));
        message.textContent = "";
      } catch (_) { if (epoch === view) { state = undefined; message.textContent = "Bookmark state unavailable. Refresh before writing."; } }
      finally { busy = false; enable(); }
    };
    const mutate = async restrict => {
      if (busy || !state || epoch !== view || (restrict && !["public", "private"].includes(restrict))) return;
      busy = true; enable();
      const args = {kind: "artwork", id};
      if (restrict) { args.restrict = restrict; args.tags = state.tags; }
      let success = false;
      try {
        const result = await request("tools/call", {name: restrict ? "pixiv_bookmark_add" : "pixiv_bookmark_remove", arguments: args}, epoch);
        if (epoch !== view || disposed) return;
        if (result.isError) throw new Error("Bookmark operation failed");
        success = true;
      } catch (_) { if (epoch === view) { state = undefined; message.textContent = "Bookmark change not confirmed. Refresh state before retrying."; } }
      finally { busy = false; enable(); }
      if (success) {
        closeOptions(); await query(); busy = true; enable();
        try { await refreshStatistics(); }
        catch (_) { if (epoch === view && !disposed) message.textContent = "Bookmark updated. Statistics unavailable; reopen details to refresh."; }
        finally { busy = false; enable(); }
      }
    };
    const save = iconAction("Bookmark", "heart", toolbar, () => {
      if (held) { held = false; return; }
      mutate(state?.bookmarked ? "" : state?.restrict === "private" ? "private" : "public");
    }); save.className = "primary-action";
    save.title = "Bookmark · hold for settings";
    save.setAttribute("aria-description", "Hold for bookmark settings, or press Shift+F10.");
    save.setAttribute("aria-expanded", "false");
    save.setAttribute("aria-haspopup", "dialog");
    save.addEventListener("pointerdown", event => {
      cancelHold(); held = false;
      if (event.button !== 0 || event.isPrimary === false || save.disabled) return;
      gesture = {x:event.clientX, y:event.clientY};
      hold = setTimeout(() => { hold = undefined; held = true; openOptions(); }, 500);
    });
    save.addEventListener("pointermove", event => {
      if (gesture && Math.hypot(event.clientX - gesture.x, event.clientY - gesture.y) > 10) { cancelHold(); held = true; }
    });
    save.addEventListener("pointerup", cancelHold);
    for (const name of ["pointercancel", "pointerleave"]) save.addEventListener(name, () => { cancelHold(); held = true; });
    save.addEventListener("contextmenu", event => { event.preventDefault(); cancelHold(); held = true; openOptions(); });
    save.addEventListener("keydown", event => {
      if (event.key === "Enter" || event.key === " ") held = false;
      if ((event.key === "F10" && event.shiftKey) || event.key === "ContextMenu") { event.preventDefault(); cancelHold(); openOptions(); }
    });
    options.addEventListener("keydown", event => { if (event.key === "Escape") { closeOptions(); save.focus?.(); } });
    animationCleanups.add(cancelHold);
    const field = text("label", "Visibility", options, "bookmark-visibility");
    const visibility = document.createElement("select"); visibility.setAttribute("aria-label", "Bookmark visibility"); field.append(visibility);
    for (const value of ["public", "private"]) { const option = text("option", value === "public" ? "Public" : "Private", visibility); option.value = value; }
    visibility.value = "public";
    const apply = action("Save changes", options, () => mutate(visibility.value)); apply.setAttribute("aria-label", "Apply bookmark settings"); apply.className = "primary-action";
    const secondary = text("div", "", options, "bookmark-options-secondary");
    const remove = iconAction("Remove bookmark", "remove", secondary, () => mutate(""));
    const refresh = iconAction("Refresh bookmark", "refresh", secondary, query); refresh.textContent = "Refresh"; refresh.append(icon("refresh"));
    const retry = iconAction("Retry bookmark state", "refresh", toolbar, query, true);
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
      const message = text("p", "Loading image…", info, "sr-only"); message.setAttribute("role", "status");
      const actions = text("div", "", info, "detail-controls");
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
        const retry = iconAction(linked ? "Refresh animation" : "Retry image", "refresh", actions, () => {
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
        }, true);
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
          } catch (_) { message.className = "error"; message.textContent = "Media content is unavailable or invalid."; }
        };
        if (animated) {
          iconAction("Play animation", "play", actions, () => show(page.content_index), true);
          iconAction("Show preview", "image", actions, () => show(page.preview_content_index), true);
        }
        show(animated ? page.preview_content_index : page.content_index);
        if (downloadFiles) downloadControl(linked ? original : {type:"resource",resource:{uri:"urn:pixiv:media:" + output.illust_id + "/" + filename,mimeType:page.mime_type,blob:original.data}}, filename, actions, epoch, notifySize);
        else if (downloadURL) downloadControl({type:"resource_link",uri:downloadURL,name:filename,mimeType:page.mime_type,size:page.size}, filename, actions, epoch, notifySize);
      } catch (_) { message.className = "error"; message.textContent = "Media content is unavailable or invalid."; }
    }
  }
  function render(result) {
    clearView();
    const epoch = view;
    if (Number.isSafeInteger(result.structuredContent?.illust_id) && result.structuredContent.illust_id > 0 && Array.isArray(result.structuredContent.pages)) {
      const {illust_id: id, title, pages} = result.structuredContent;
      // Static media reuses the full detail page; animations keep their embedded playback view.
      if (serverTools && pages.length && !pages.some(page => Number.isSafeInteger(page?.preview_content_index))) {
        if (Number.isSafeInteger(pages[0]?.page) && pages[0].page > 0) remember("page:" + id, pages[0].page, 8);
        openDetail({id: String(id), type: "illust", title});
      } else renderMedia(result, epoch);
      return;
    }
    const records = result.structuredContent?.records;
    if (!Array.isArray(records)) { status.textContent = "No structured records; use the tool result in chat."; return; }
    // A single artwork goes straight to detail; without tool calls or a valid ID detail cannot load, so keep the card.
    const only = records.length === 1 ? records[0] : undefined;
    if (serverTools && ["illust", "manga", "ugoira"].includes(only?.type) && Number.isSafeInteger(Number(only.id)) && Number(only.id) > 0) { openDetail(only); return; }
    status.textContent = result.isError ? "Tool failed or returned partial results." : records.length ? "" : "No results.";
    if (records.length === 1 && ["illust", "manga", "ugoira"].includes(records[0]?.type)) cards.className = "single-artwork";
    if (records.length > 1) { cards.className = "gallery-results"; cards.setAttribute("tabindex", "0"); }
    if (result.isError) status.className = "sr-only";
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
    // Failures usually share a cause (expired capabilities); one retry renews every failed card in one batch.
    const thumbs = [];
    const retryFailed = () => {
      const failed = thumbs.filter(thumb => thumb.media.failed());
      if (!serverTools || !failed.length) return;
      let batch;
      const source = () => batch ??= request("tools/call", {name:"pixiv_show_gallery", arguments:{illust_ids:failed.map(thumb => thumb.id), page:1, quality:"thumbnail"}}, epoch);
      for (const thumb of failed) thumb.media.renew(source);
    };
    for (const record of records) {
      const card = document.createElement("article"); card.className = "artwork-card";
      cards.append(card);
      if (!record || typeof record !== "object" || Array.isArray(record)) { text("p", "Invalid record — inspect the tool result in chat.", card, "error"); continue; }
      if (["illust", "manga", "ugoira"].includes(record.type)) {
        const media = proxyImage(record, card, currentThumbnails, epoch, 1, "thumbnail", () => openDetail(record, result), undefined, undefined, retryFailed);
        thumbs.push({id: Number(record.id), media});
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
        clearView(); clearCache(); status.textContent = "Loading results…"; status.className = "sr-only"; break;
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
  request("ui/initialize", {appInfo: {name: "pixiv-gallery", version: "1"}, appCapabilities: {availableDisplayModes: ["inline", "fullscreen"]}, protocolVersion: "2026-01-26"})
    .then(result => {
      if (disposed) return;
      if (result.protocolVersion !== "2026-01-26") throw new Error("Unsupported bridge protocol");
      serverTools = Boolean(result.hostCapabilities?.serverTools);
      downloadFiles = Boolean(result.hostCapabilities?.downloadFile);
      chatMessages = Boolean(result.hostCapabilities?.message?.text);
      applyContext(result.hostContext);
      ready = true;
      send({method: "ui/notifications/initialized", params: {}});
      status.textContent = "Waiting for tool results…";
      if (typeof ResizeObserver === "function") {
        resize = new ResizeObserver(notifySize);
        resize.observe(document.body);
        window.addEventListener("pagehide", () => resize.disconnect(), {once: true});
      }
    }).catch(() => { if (!disposed) { status.className = ""; status.textContent = "Gallery bridge unavailable. Use structured results and pixiv_artwork_media in chat."; } });
})();
