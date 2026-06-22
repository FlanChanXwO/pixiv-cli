"use strict";

const loginButton = document.querySelector("#login-button");
const clearButton = document.querySelector("#clear-button");
const manualButton = document.querySelector("#manual-button");
const manualCode = document.querySelector("#manual-code");
const statusBox = document.querySelector("#status");
const tokenSection = document.querySelector("#token-section");
const tokenOutput = document.querySelector("#token-output");
const copyButton = document.querySelector("#copy-button");

loginButton.addEventListener("click", async () => {
  await withBusy(loginButton, "正在打开...", async () => {
    const response = await sendMessage({ type: "start-login" });
    if (!response.ok) {
      showError(response.error);
      return;
    }
    showStatus("Pixiv 登录页已打开。请在新标签页完成登录，扩展会自动捕获 OAuth code。");
  });
});

clearButton.addEventListener("click", async () => {
  const response = await sendMessage({ type: "clear-state" });
  if (!response.ok) {
    showError(response.error);
    return;
  }
  tokenSection.hidden = true;
  tokenOutput.value = "";
  showStatus("临时状态已清理。");
});

manualButton.addEventListener("click", async () => {
  await withBusy(manualButton, "正在换取...", async () => {
    const response = await sendMessage({ type: "exchange-manual-code", value: manualCode.value });
    if (!response.ok) {
      showError(response.error);
      return;
    }
    showStatus("已提交 OAuth code，结果页会自动打开。");
  });
});

copyButton.addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(tokenOutput.value);
    showStatus("已复制 refresh_token。");
  } catch (error) {
    showError(`复制失败，请手动选择文本复制：${error.message || error}`);
  }
});

initialize();

async function initialize() {
  await continueActiveTab();
  await consumeToken();
}

async function continueActiveTab() {
  const response = await sendMessage({ type: "continue-active-tab" });
  if (!response.ok) {
    showError(response.error);
    return;
  }
  if (response.status) {
    showStatus(response.status);
  }
}

async function consumeToken() {
  const response = await sendMessage({ type: "consume-token" });
  if (!response.ok) {
    showError(response.error);
    return;
  }
  if (response.error) {
    showError(response.error);
  }
  if (response.token) {
    tokenOutput.value = response.token;
    tokenSection.hidden = false;
    showStatus("refresh_token 已生成。扩展不会持久保存它，请复制后妥善保管。");
  } else if (location.hash === "#token") {
    showStatus("没有可显示的 token。请重新点击开始登录。");
  } else if (!statusBox.textContent) {
    showStatus("准备就绪。");
  }
}

function sendMessage(message) {
  return chrome.runtime.sendMessage(message);
}

async function withBusy(button, label, fn) {
  const original = button.textContent;
  button.disabled = true;
  button.textContent = label;
  try {
    await fn();
  } finally {
    button.disabled = false;
    button.textContent = original;
  }
}

function showStatus(message) {
  statusBox.classList.remove("error");
  statusBox.textContent = message;
}

function showError(message) {
  statusBox.classList.add("error");
  statusBox.textContent = message;
}
