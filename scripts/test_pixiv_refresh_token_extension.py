#!/usr/bin/env python3
"""Offline tests for the Pixiv RefreshToken browser extension."""

import json
import pathlib
import re
import subprocess
import textwrap
import unittest


ROOT = pathlib.Path(__file__).resolve().parent
EXTENSION = ROOT / "pixiv-refresh-token-extension"
MANIFEST = EXTENSION / "manifest.json"
BACKGROUND = EXTENSION / "background.js"
INDEX = EXTENSION / "index.html"
POPUP_SCRIPT = EXTENSION / "index.js"


class RefreshTokenExtensionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
        cls.background = BACKGROUND.read_text(encoding="utf-8")
        cls.index = INDEX.read_text(encoding="utf-8")
        cls.popup_script = POPUP_SCRIPT.read_text(encoding="utf-8")

    def test_static_files_exist(self):
        for name in ["manifest.json", "background.js", "index.html", "index.js", "styles.css", "post_redirect.js"]:
            self.assertTrue((EXTENSION / name).exists(), name)

    def test_manifest_v3_minimal_permissions(self):
        self.assertEqual(self.manifest["manifest_version"], 3)
        self.assertEqual(set(self.manifest["permissions"]), {"storage", "tabs", "webNavigation", "webRequest"})
        self.assertEqual(set(self.manifest["host_permissions"]), {
            "https://app-api.pixiv.net/*",
            "https://accounts.pixiv.net/*",
            "https://oauth.secure.pixiv.net/*",
        })
        forbidden = {"cookies", "webRequestBlocking", "<all_urls>"}
        self.assertTrue(forbidden.isdisjoint(set(self.manifest["permissions"])))
        self.assertTrue(forbidden.isdisjoint(set(self.manifest["host_permissions"])))
        self.assertEqual(self.manifest["content_scripts"][0]["matches"], ["https://accounts.pixiv.net/post-redirect*"])

    def test_oauth_constants(self):
        self.assertIn('const LOGIN_URL = "https://app-api.pixiv.net/web/v1/login";', self.background)
        self.assertIn('const TOKEN_URL = "https://oauth.secure.pixiv.net/auth/token";', self.background)
        self.assertIn('const REDIRECT_URI = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback";', self.background)
        self.assertIn('const CLIENT_ID = "MOBrBDSOnz6cTIM6GAl6Ytjj";', self.background)
        self.assertIn('url.searchParams.set("client", "pixiv-android");', self.background)
        for header in ['"User-Agent"', '"App-OS"', '"App-OS-Version"', '"App-Version"']:
            self.assertIn(header, self.background)

    def test_refresh_token_is_not_persisted(self):
        storage_local_or_sync = re.findall(r"chrome\.storage\.(?:local|sync)", self.background)
        self.assertEqual(storage_local_or_sync, [])
        self.assertEqual(self.background.count("await chrome.storage.session.set({ [STORAGE_TOKEN]: token });"), 1)
        self.assertIn("await chrome.storage.session.remove([STORAGE_TOKEN]);", self.background)
        self.assertNotIn("chrome.cookies", self.background)

    def test_callback_parsing_with_node(self):
        js = textwrap.dedent(
            f"""
            global.chrome = {{
              runtime: {{ onMessage: {{ addListener() {{}} }} }},
              webNavigation: {{ onBeforeNavigate: {{ addListener() {{}} }} }},
              webRequest: {{
                onBeforeRequest: {{ addListener() {{}} }},
                onBeforeRedirect: {{ addListener() {{}} }},
              }},
            }};
            global.crypto = {{
              getRandomValues() {{ throw new Error("not needed"); }},
              subtle: {{}},
            }};
            global.TextEncoder = class TextEncoder {{}};
            global.btoa = (value) => Buffer.from(value, "binary").toString("base64");
            const {{ extractOAuthCode, isPixivStartURL }} = require({json.dumps(str(BACKGROUND))});
            const cases = [
              ["https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?state=s&code=c12345678901234567890", "c12345678901234567890", "s"],
              ["pixiv://account/login?code=VBWW8GLNDFeZqQuBbmnrxPokNIn20plgwgIoAUmdy7U&via=login", "VBWW8GLNDFeZqQuBbmnrxPokNIn20plgwgIoAUmdy7U", ""],
              ["https://accounts.pixiv.net/post-redirect?return_to=https%3A%2F%2Fapp-api.pixiv.net%2Fweb%2Fv1%2Fusers%2Fauth%2Fpixiv%2Fcallback%3Fstate%3Ds2%26code%3Dnested12345678901234567890", "nested12345678901234567890", "s2"],
              ["rawCodeValue_12345678901234567890", "rawCodeValue_12345678901234567890", ""],
            ];
            for (const [input, expectedCode, expectedState] of cases) {{
              const actual = extractOAuthCode(input);
              if (actual.code !== expectedCode || actual.state !== expectedState) {{
                throw new Error(JSON.stringify({{ input, actual, expectedCode, expectedState }}));
              }}
            }}
            if (extractOAuthCode("").code !== "") {{
              throw new Error("empty input should not return a code");
            }}
            if (!isPixivStartURL("https://app-api.pixiv.net/web/v1/users/auth/pixiv/start?code_challenge=abc")) {{
              throw new Error("pixiv start URL should be detected");
            }}
            if (isPixivStartURL("https://accounts.pixiv.net/post-redirect?return_to=https%3A%2F%2Fapp-api.pixiv.net%2Fweb%2Fv1%2Fusers%2Fauth%2Fpixiv%2Fstart")) {{
              throw new Error("post-redirect URL should not be treated as start endpoint");
            }}
            """
        )
        subprocess.run(["node", "-e", js], check=True)

    def test_popup_mentions_manual_fallback(self):
        self.assertIn("manual-code", self.index)
        self.assertIn("手动换取 token", self.index)
        self.assertIn('type: "continue-active-tab"', self.popup_script)
        self.assertNotIn("extractPostRedirectTarget", self.background)
        self.assertNotIn("continuePostRedirect", self.background)


if __name__ == "__main__":
    unittest.main()
