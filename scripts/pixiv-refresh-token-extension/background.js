"use strict";

const STORAGE_VERIFIER = "pixiv_refresh_token_extension_pkce_verifier";
const STORAGE_STATE = "pixiv_refresh_token_extension_oauth_state";
const STORAGE_TOKEN = "pixiv_refresh_token_extension_result_token";
const STORAGE_ERROR = "pixiv_refresh_token_extension_last_error";

const LOGIN_URL = "https://app-api.pixiv.net/web/v1/login";
const TOKEN_URL = "https://oauth.secure.pixiv.net/auth/token";
const REDIRECT_URI = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback";
const CLIENT_ID = "MOBrBDSOnz6cTIM6GAl6Ytjj";
const CLIENT_SECRET = "lsyM0L2M6vWypx4Y";
const USER_AGENT = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)";
const APP_OS = "android";
const APP_OS_VERSION = "11";
const APP_VERSION = "5.0.234";
const observedCodes = new Set();

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  const type = message && message.type;
  if (type === "start-login") {
    startLogin().then(sendResponse).catch((error) => sendResponse(errorResult(error)));
    return true;
  }
  if (type === "exchange-manual-code") {
    exchangeManualCode(message.value).then(sendResponse).catch((error) => sendResponse(errorResult(error)));
    return true;
  }
  if (type === "consume-token") {
    consumeToken().then(sendResponse).catch((error) => sendResponse(errorResult(error)));
    return true;
  }
  if (type === "clear-state") {
    clearState().then(sendResponse).catch((error) => sendResponse(errorResult(error)));
    return true;
  }
  if (type === "continue-active-tab") {
    continueActiveTab().then(sendResponse).catch((error) => sendResponse(errorResult(error)));
    return true;
  }
  if (type === "post-redirect-submitted") {
    rememberError(new Error("检测到 Pixiv 登录中转页，已尝试提交页面表单并等待 OAuth callback。")).then(() => sendResponse({ ok: true })).catch((error) => sendResponse(errorResult(error)));
    return true;
  }
  return false;
});

chrome.webNavigation.onBeforeNavigate.addListener((details) => {
  if (details.frameId !== 0 || !details.url) {
    return;
  }
  handleOAuthURL(details.url, details.tabId);
}, {
  url: [
    { hostEquals: "app-api.pixiv.net" },
    { hostEquals: "accounts.pixiv.net" }
  ]
});

chrome.webRequest.onBeforeRequest.addListener((details) => {
  handleOAuthURL(details.url, details.tabId);
}, {
  urls: [
    "https://app-api.pixiv.net/*",
    "https://accounts.pixiv.net/*"
  ]
});

chrome.webRequest.onBeforeRedirect.addListener((details) => {
  handleOAuthURL(details.redirectUrl || details.url, details.tabId);
}, {
  urls: [
    "https://app-api.pixiv.net/*",
    "https://accounts.pixiv.net/*"
  ]
});

async function startLogin() {
  const verifier = randomBase64URL(32);
  const challenge = await sha256Base64URL(verifier);
  const state = randomBase64URL(24);
  await chrome.storage.session.set({
    [STORAGE_VERIFIER]: verifier,
    [STORAGE_STATE]: state,
  });
  await chrome.storage.session.remove([STORAGE_TOKEN, STORAGE_ERROR]);

  const tab = await chrome.tabs.create({ url: buildLoginURL(challenge, state) });
  return { ok: true, tabId: tab.id, login_url: tab.url };
}

async function exchangeManualCode(input) {
  const extracted = extractOAuthCode(input || "");
  if (!extracted.code) {
    throw new Error("没有找到 code。请粘贴完整 callback URL、pixiv:// URL，或只粘贴 code 值。");
  }
  return handleOAuthCode(extracted.code, extracted.state);
}

async function handleOAuthCode(code, callbackState, tabId) {
  const stateValues = await chrome.storage.session.get([STORAGE_VERIFIER, STORAGE_STATE]);
  const verifier = stateValues[STORAGE_VERIFIER] || "";
  const expectedState = stateValues[STORAGE_STATE] || "";
  if (!verifier) {
    throw new Error("找不到临时 PKCE verifier。请重新点击开始登录。");
  }
  if (expectedState && callbackState && expectedState !== callbackState) {
    throw new Error("OAuth state 不匹配。请重新点击开始登录。");
  }

  const token = await exchangeCode(code, verifier);
  await chrome.storage.session.set({ [STORAGE_TOKEN]: token });
  await chrome.storage.session.remove([STORAGE_VERIFIER, STORAGE_STATE, STORAGE_ERROR]);

  const resultURL = chrome.runtime.getURL("index.html#token");
  if (typeof tabId === "number" && tabId >= 0) {
    await chrome.tabs.update(tabId, { url: resultURL });
  } else {
    await chrome.tabs.create({ url: resultURL });
  }
  return { ok: true };
}

async function consumeToken() {
  const values = await chrome.storage.session.get([STORAGE_TOKEN, STORAGE_ERROR]);
  const token = values[STORAGE_TOKEN] || "";
  const error = values[STORAGE_ERROR] || "";
  await chrome.storage.session.remove([STORAGE_TOKEN]);
  return { ok: true, token, error };
}

async function clearState() {
  await chrome.storage.session.remove([STORAGE_VERIFIER, STORAGE_STATE, STORAGE_TOKEN, STORAGE_ERROR]);
  return { ok: true };
}

async function continueActiveTab() {
  const tabs = await chrome.tabs.query({ active: true, currentWindow: true });
  const tab = tabs[0];
  if (!tab || typeof tab.id !== "number" || !tab.url) {
    return { ok: true, handled: false, status: "" };
  }
  if (handleOAuthURL(tab.url, tab.id)) {
    return { ok: true, handled: true, status: "已从当前标签页 URL 捕获 OAuth code。" };
  }
  if (isPixivStartURL(tab.url)) {
    const message = "当前标签页是 Pixiv /start 的 GET 错误页。请回到扩展弹窗重新点击开始登录；扩展现在会等待 Pixiv 中转页表单和 HTTP redirect，不再直接打开这个端点。";
    await rememberError(new Error(message));
    return { ok: true, handled: false, status: message };
  }
  return { ok: true, handled: false, status: "" };
}

async function rememberError(error) {
  await chrome.storage.session.set({ [STORAGE_ERROR]: errorMessage(error) });
}

function buildLoginURL(challenge, state) {
  const url = new URL(LOGIN_URL);
  url.searchParams.set("code_challenge", challenge);
  url.searchParams.set("code_challenge_method", "S256");
  url.searchParams.set("client", "pixiv-android");
  url.searchParams.set("state", state);
  return url.toString();
}

async function exchangeCode(code, verifier) {
  const body = new URLSearchParams();
  body.set("client_id", CLIENT_ID);
  body.set("client_secret", CLIENT_SECRET);
  body.set("code", code);
  body.set("code_verifier", verifier);
  body.set("grant_type", "authorization_code");
  body.set("include_policy", "true");
  body.set("redirect_uri", REDIRECT_URI);

  const response = await fetch(TOKEN_URL, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "User-Agent": USER_AGENT,
      "App-OS": APP_OS,
      "App-OS-Version": APP_OS_VERSION,
      "App-Version": APP_VERSION,
    },
    body: body.toString(),
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`token endpoint returned HTTP ${response.status}: ${text}`);
  }
  const parsed = JSON.parse(text || "{}");
  const fields = parsed.response && typeof parsed.response === "object" ? parsed.response : parsed;
  if (!fields.refresh_token) {
    throw new Error("token endpoint response did not include refresh_token");
  }
  return fields.refresh_token;
}

function extractOAuthCode(input) {
  const raw = String(input || "").trim().replace(/^['"]|['"]$/g, "");
  if (!raw) {
    return { code: "", state: "" };
  }
  if (/^[A-Za-z0-9_-]{20,}$/.test(raw) && !raw.includes("=") && !raw.includes("?")) {
    return { code: raw, state: "" };
  }

  const queue = [raw];
  const seen = new Set();
  while (queue.length > 0) {
    const candidate = queue.shift();
    if (!candidate || seen.has(candidate)) {
      continue;
    }
    seen.add(candidate);

    const parsed = parseURL(candidate);
    if (!parsed) {
      continue;
    }
    const code = parsed.searchParams.get("code") || "";
    const state = parsed.searchParams.get("state") || "";
    if (code) {
      return { code, state };
    }
    const returnTo = parsed.searchParams.get("return_to");
    if (returnTo) {
      queue.push(returnTo);
    }
  }
  return { code: "", state: "" };
}

function handleOAuthURL(input, tabId) {
  const extracted = extractOAuthCode(input);
  if (!extracted.code) {
    return false;
  }
  if (observedCodes.has(extracted.code)) {
    return true;
  }
  observedCodes.add(extracted.code);
  handleOAuthCode(extracted.code, extracted.state, tabId).catch((error) => {
    rememberError(error);
  });
  return true;
}

function isPixivStartURL(input) {
  const parsed = parseURL(input);
  return Boolean(parsed && parsed.hostname === "app-api.pixiv.net" && parsed.pathname === "/web/v1/users/auth/pixiv/start");
}

function parseURL(value) {
  try {
    return new URL(value);
  } catch (_) {
    return null;
  }
}

function randomBase64URL(size) {
  const raw = new Uint8Array(size);
  crypto.getRandomValues(raw);
  return base64URL(raw);
}

async function sha256Base64URL(value) {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return base64URL(new Uint8Array(digest));
}

function base64URL(bytes) {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function errorResult(error) {
  return { ok: false, error: errorMessage(error) };
}

function errorMessage(error) {
  return error && error.message ? error.message : String(error);
}

if (typeof module !== "undefined") {
  module.exports = {
    buildLoginURL,
    extractOAuthCode,
    isPixivStartURL,
  };
}
