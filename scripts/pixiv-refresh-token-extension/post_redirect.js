"use strict";

(function () {
  const forms = Array.from(document.forms || []);
  if (forms.length === 0) {
    chrome.runtime.sendMessage({
      type: "post-redirect-submitted",
      form_found: false,
      url: location.href,
    });
    return;
  }

  const form = forms.find((item) => item.action && item.action.includes("/web/v1/users/auth/pixiv/start")) || forms[0];
  chrome.runtime.sendMessage({
    type: "post-redirect-submitted",
    form_found: true,
    method: form.method || "get",
    action: form.action || "",
    url: location.href,
  });
  HTMLFormElement.prototype.submit.call(form);
})();
