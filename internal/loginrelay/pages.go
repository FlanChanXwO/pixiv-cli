package loginrelay

import (
	"bytes"
	"html/template"
	"io"
	"io/fs"

	"github.com/FlanChanXwO/pixiv-cli/frontend"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

var templatesFS = frontend.MustLoad().Login

// 样式只来自随程序嵌入的源码，运行时不读取本地文件。
var stylesheet = func() string {
	value, err := fs.ReadFile(templatesFS, "page.css")
	if err != nil {
		panic(err)
	}
	return string(value)
}()

type pageData struct {
	Language   i18n.Locale
	Stylesheet template.CSS
	Title      string
	BodyClass  string
	LoginURL   string
	Success    bool
	Heading    string
	Message    string
}

var (
	manualTemplate   = mustParse("manual.html")
	callbackTemplate = mustParse("callback-relay.html")
	resultTemplate   = mustParse("result.html")
)

func mustParse(name string) *template.Template {
	return template.Must(template.ParseFS(templatesFS, "layout.html", name))
}

// mustRender 只用于不含运行期数据的嵌入页面；资源损坏是构建不变量，应在启动时显露。
func mustRender(tmpl *template.Template, data pageData) []byte {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic("loginrelay: render " + tmpl.Name() + ": " + err.Error())
	}
	return out.Bytes()
}

func baseData(locale i18n.Locale) pageData {
	return pageData{Stylesheet: template.CSS(stylesheet), Title: "pixiv-cli", Language: locale}
}

func bridgeData(locale i18n.Locale) pageData {
	data := baseData(locale)
	data.BodyClass = "pv-bridge"
	return data
}

func resultData(success bool, locale i18n.Locale) pageData {
	data := baseData(locale)
	data.Success = success
	if success {
		data.BodyClass = "pv-final"
		data.Heading = locale.Text("login.success_heading")
		data.Message = locale.Text("login.success_message")
		return data
	}
	data.BodyClass = "pv-final pv-bad"
	data.Heading = locale.Text("login.failure_heading")
	data.Message = locale.Text("login.failure_message")
	return data
}

// WriteManual 渲染包含 Pixiv 登录链接和手动回填表单的页面。
func WriteManual(w io.Writer, loginURL string, locale i18n.Locale) error {
	data := baseData(locale)
	data.BodyClass = "pv-form-page"
	data.LoginURL = loginURL
	return manualTemplate.Execute(w, data)
}

// WriteCallbackRelay 渲染将 pixiv:// fragment 安全地提交到本地 /manual 路由的页面。
func WriteCallbackRelay(w io.Writer, locale i18n.Locale) error {
	_, err := w.Write(fixedPages[locale.String()].callback)
	return err
}

// writeResult 渲染固定的登录完成页面。失败文案不包含 OAuth 或本地存储错误细节。
func writeResult(w io.Writer, success bool, locale i18n.Locale) error {
	if success {
		_, err := w.Write(fixedPages[locale.String()].success)
		return err
	}
	_, err := w.Write(fixedPages[locale.String()].failure)
	return err
}

// 固定页按语言预渲染，多个浏览器共享不可变资源。
var fixedPages = func() map[string]struct{ callback, success, failure []byte } {
	pages := make(map[string]struct{ callback, success, failure []byte })
	for _, locale := range []i18n.Locale{i18n.English, i18n.Chinese} {
		pages[locale.String()] = struct{ callback, success, failure []byte }{
			mustRender(callbackTemplate, bridgeData(locale)),
			mustRender(resultTemplate, resultData(true, locale)),
			mustRender(resultTemplate, resultData(false, locale)),
		}
	}
	return pages
}()
