// Package loginpage 负责渲染 CLI 登录流程临时 HTTP 服务使用的页面。
//
// 页面与路由、OAuth 会话分离：本包不读取账号数据，也不处理表单提交，只渲染经
// html/template 转义后的展示内容。模板嵌入 binary，使手工安装的 CLI 不依赖外部文件。
package loginpage

import (
	"bytes"
	"embed"
	"html/template"
	"io"
)

const (
	pageTitle      = "pixiv-cli"
	successHeading = "Login successful"
	successMessage = "Login completed. You can close this page and return to the terminal."
	failureHeading = "Login failed"
	failureMessage = "Login could not be completed. Return to the terminal to view details or try again."
)

//go:embed templates/*.html assets/page.css
var templatesFS embed.FS

// 页面模板与样式均随 CLI 二进制分发；不依赖运行环境中的外部静态文件。
// 样式为本包维护的静态资源，故以 template.CSS 传入模板，避免被当作不可信内容替换。
//
//go:embed assets/page.css
var stylesheet string

type pageData struct {
	Stylesheet template.CSS
	Title      string
	BodyClass  string
	LoginURL   string
	Success    bool
	Heading    string
	Message    string
}

var (
	manualTemplate   = mustParse("templates/manual.html")
	callbackTemplate = mustParse("templates/callback-relay.html")
	resultTemplate   = mustParse("templates/result.html")

	callbackPage      = mustRender(callbackTemplate, bridgeData())
	successResultPage = mustRender(resultTemplate, resultData(true))
	failureResultPage = mustRender(resultTemplate, resultData(false))
)

func mustParse(name string) *template.Template {
	return template.Must(template.ParseFS(templatesFS, "templates/layout.html", name))
}

// mustRender 只用于不含运行期数据的嵌入页面；资源损坏是构建不变量，应在启动时显露。
func mustRender(tmpl *template.Template, data pageData) []byte {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic("loginpage: render " + tmpl.Name() + ": " + err.Error())
	}
	return out.Bytes()
}

func baseData() pageData {
	return pageData{Stylesheet: template.CSS(stylesheet), Title: pageTitle}
}

func bridgeData() pageData {
	data := baseData()
	data.BodyClass = "pv-bridge"
	return data
}

func resultData(success bool) pageData {
	data := baseData()
	data.Success = success
	if success {
		data.BodyClass = "pv-final"
		data.Heading = successHeading
		data.Message = successMessage
		return data
	}
	data.BodyClass = "pv-final pv-bad"
	data.Heading = failureHeading
	data.Message = failureMessage
	return data
}

// WriteManual 渲染包含 Pixiv 登录链接和手动回填表单的页面。
func WriteManual(w io.Writer, loginURL string) error {
	data := baseData()
	data.BodyClass = "pv-form-page"
	data.LoginURL = loginURL
	return manualTemplate.Execute(w, data)
}

// WriteCallbackRelay 渲染将 pixiv:// fragment 安全地提交到本地 /manual 路由的页面。
func WriteCallbackRelay(w io.Writer) error {
	_, err := w.Write(callbackPage)
	return err
}

// WriteResult 渲染固定的登录完成页面。失败文案不包含 OAuth 或本地存储错误细节。
func WriteResult(w io.Writer, success bool) error {
	if success {
		_, err := w.Write(successResultPage)
		return err
	}
	_, err := w.Write(failureResultPage)
	return err
}
