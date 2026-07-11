package public

import (
	"embed"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/database/dbcore"
	"github.com/monitor-monitor/monitor/database/models"
	"github.com/monitor-monitor/monitor/pkg/config"
)

var startTimeUnix = time.Now().Unix()

//go:embed defaultTheme vpsTheme
var PublicFS embed.FS

// 常量定义
const (
	DataDir            = "./data"
	ThemesDir          = "theme"
	FaviconFile        = "favicon.ico"
	DefaultTheme       = "default"
	VpsTheme           = "VPS"
	LanguageCookieName = "language"

	// 主题内部结构定义
	DistDir   = "dist"       // 静态资源存放目录
	IndexFile = "index.html" // 相对于 DistDir
)

func init() {
	_ = os.MkdirAll("./data/theme", 0755)
}

func normalizeHTMLLanguage(language string) string {
	language = strings.TrimSpace(strings.ReplaceAll(language, "_", "-"))
	if len(language) < 2 || len(language) > 32 {
		return ""
	}

	for _, r := range language {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return ""
	}

	return language
}

func replaceHTMLLanguage(htmlStr, language string) string {
	language = normalizeHTMLLanguage(language)
	if language == "" {
		return htmlStr
	}

	replacements := []struct {
		old string
		new string
	}{
		{`<html lang="en">`, `<html lang="` + language + `">`},
		{`<html lang='en'>`, `<html lang='` + language + `'>`},
		{`<html>`, `<html lang="` + language + `">`},
	}

	for _, replacement := range replacements {
		if strings.Contains(htmlStr, replacement.old) {
			return strings.Replace(htmlStr, replacement.old, replacement.new, 1)
		}
	}

	return htmlStr
}

func hasValidSession(c *gin.Context) bool {
	sessionToken, err := c.Cookie("session_token")
	if err != nil || strings.TrimSpace(sessionToken) == "" {
		return false
	}

	var session models.Session
	if err := dbcore.GetDBInstance().Where("session = ?", sessionToken).First(&session).Error; err != nil {
		return false
	}
	return !time.Now().After(session.Expires.ToTime())
}

func getFaviconTimestamp() string {
	localFavicon := filepath.Join(DataDir, FaviconFile)
	if info, err := os.Stat(localFavicon); err == nil {
		return strconv.FormatInt(info.ModTime().Unix(), 10)
	}
	return strconv.FormatInt(startTimeUnix, 10)
}

func serveLoginPage(c *gin.Context, siteName string) {
	escapedSiteName := html.EscapeString(siteName)
	favTimestamp := getFaviconTimestamp()
	page := `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <link rel="icon" href="/favicon.ico?t=` + favTimestamp + `" />
  <title>` + escapedSiteName + `</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #07111f;
      --panel: rgba(15, 26, 43, .92);
      --line: rgba(148, 163, 184, .28);
      --text: #f8fafc;
      --muted: #94a3b8;
      --green: #16a34a;
      --green-hover: #22c55e;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      background:
        linear-gradient(135deg, rgba(34, 197, 94, .14), transparent 34%),
        radial-gradient(circle at 75% 18%, rgba(56, 189, 248, .14), transparent 32%),
        var(--bg);
      color: var(--text);
    }
    main {
      width: min(420px, calc(100vw - 32px));
      padding: 34px;
      border: 1px solid var(--line);
      border-radius: 22px;
      background: var(--panel);
      box-shadow: 0 24px 70px rgba(0, 0, 0, .34);
      backdrop-filter: blur(18px);
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 16px;
      margin-bottom: 30px;
    }
    .avatar-button {
      width: 58px;
      height: 58px;
      padding: 0;
      margin: 0;
      border: 0;
      border-radius: 999px;
      background: transparent;
      cursor: pointer;
      box-shadow: none;
    }
    .avatar-button:hover { background: transparent; }
    .brand img {
      width: 58px;
      height: 58px;
      border-radius: 999px;
      object-fit: cover;
      border: 1px solid rgba(255, 255, 255, .16);
      display: block;
    }
    h1 {
      margin: 0;
      font-size: 32px;
      line-height: 1.1;
      letter-spacing: 0;
    }
    label {
      display: block;
      margin: 18px 0 8px;
      font-size: 16px;
      font-weight: 700;
    }
    input {
      width: 100%;
      height: 54px;
      border: 1px solid rgba(148, 163, 184, .32);
      border-radius: 14px;
      padding: 0 16px;
      background: rgba(2, 6, 23, .36);
      color: var(--text);
      font: inherit;
      outline: none;
    }
    input:focus {
      border-color: var(--green);
      box-shadow: 0 0 0 3px rgba(34, 197, 94, .2);
    }
    button {
      width: 100%;
      height: 56px;
      margin-top: 26px;
      border: 0;
      border-radius: 14px;
      background: var(--green);
      color: white;
      font: inherit;
      font-size: 18px;
      font-weight: 800;
      cursor: pointer;
    }
    button:hover { background: var(--green-hover); }
    button:disabled { cursor: wait; opacity: .72; }
    .error {
      min-height: 22px;
      margin-top: 14px;
      color: #fca5a5;
      font-size: 14px;
    }
  </style>
</head>
<body>
  <main>
    <div class="brand">
      <img src="/images/ethan-avatar.png" alt="logo" onerror="this.style.display='none'" />
      <h1>` + escapedSiteName + `</h1>
    </div>
    <form id="login-form">
      <label for="username">用户名</label>
      <input id="username" name="username" autocomplete="username" autofocus />
      <label for="password">密码</label>
      <input id="password" name="password" type="password" autocomplete="current-password" />
      <button id="submit" type="submit">登录</button>
      <div id="error" class="error" role="alert"></div>
    </form>
  </main>
  <script>
    const form = document.getElementById('login-form');
    const button = document.getElementById('submit');
    const error = document.getElementById('error');

    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      error.textContent = '';
      button.disabled = true;
      try {
        const response = await fetch('/api/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({
            username: form.username.value.trim(),
            password: form.password.value
          })
        });
        if (!response.ok) throw new Error('用户名或密码不正确');
        const target = location.pathname.startsWith('/admin') ? '/admin' : '/';
        location.replace(target);
      } catch (err) {
        error.textContent = err && err.message ? err.message : '登录失败';
      } finally {
        button.disabled = false;
      }
    });
  </script>
</body>
</html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

// isSafePath 验证路径是否在指定的基础目录内，防止路径穿透攻击
func isSafePath(basePath, targetPath string) bool {
	// 获取基础目录的绝对路径
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return false
	}

	// 清理目标路径，移除 ../ 等
	cleanTarget := filepath.Clean(targetPath)

	// 拼接完整路径
	fullPath := filepath.Join(absBase, cleanTarget)

	// 获取绝对路径
	absTarget, err := filepath.Abs(fullPath)
	if err != nil {
		return false
	}

	// 检查目标路径是否以基础路径开头
	// 使用 filepath.Rel 更可靠地检查路径关系
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}

	// 如果相对路径以 .. 开头，说明目标在基础目录之外
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

// Static 注册静态资源和 SPA 路由处理
func Static(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc)) {
	// 初始化嵌入式文件系统，指向 defaultTheme 根目录
	// 假设 defaultTheme 内部结构也是: dist/, theme.json 等
	defaultThemeFS, err := fs.Sub(PublicFS, "defaultTheme")
	if err != nil {
		panic("you may forget to put dist of frontend to web/public/defaultTheme/dist")
	}
	vpsThemeFS, err := fs.Sub(PublicFS, "vpsTheme")
	if err != nil {
		panic("you may forget to put dist of VPS theme to web/public/vpsTheme/dist")
	}

	getConfig := func() map[string]any {
		cfg, _ := config.GetMany(map[string]any{
			config.SitenameKey: "VPS Monitor",
			config.ThemeKey:    VpsTheme,
		})
		return cfg
	}

	// 核心逻辑：获取文件内容
	// filePath: 相对于主题根目录的路径 (例如 "theme.json" 或 "dist/assets/a.js")
	// 返回: content, contentType, exists
	getFileContent := func(themeID string, relativePath string) ([]byte, string, bool) {
		cleanPath := strings.TrimPrefix(relativePath, "/")

		cleanPath = filepath.Clean(cleanPath)
		normalizedPath := filepath.ToSlash(cleanPath)
		if normalizedPath == filepath.ToSlash(filepath.Join("images", "ethan-avatar.png")) ||
			normalizedPath == filepath.ToSlash(filepath.Join(DistDir, "images", "ethan-avatar.png")) {
			localAvatar := filepath.Join(DataDir, "avatar", "ethan-avatar.png")
			if info, err := os.Stat(localAvatar); err == nil && !info.IsDir() {
				if content, err := os.ReadFile(localAvatar); err == nil {
					return content, "image/png", true
				}
			}
		}

		if themeID != DefaultTheme && themeID != VpsTheme {
			if strings.Contains(themeID, "..") || strings.Contains(themeID, "/") || strings.Contains(themeID, "\\") {
				return nil, "", false
			}

			themeBasePath := filepath.Join(DataDir, ThemesDir, themeID)

			if !isSafePath(themeBasePath, cleanPath) {
				return nil, "", false
			}

			localPath := filepath.Join(themeBasePath, cleanPath)
			// 检查文件是否存在且不是目录
			if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
				content, err := os.ReadFile(localPath)
				if err == nil {
					return content, mime.TypeByExtension(filepath.Ext(localPath)), true
				}
			}
			// 本地文件不存在，或读取失败 -> 继续向下回退
		}

		// 2. 尝试从嵌入式 VPS 主题读取
		// fs.ReadFile 处理 embed 路径时使用 "/"
		embedPath := filepath.ToSlash(cleanPath)

		if strings.Contains(embedPath, "..") {
			return nil, "", false
		}

		if themeID == VpsTheme {
			if content, err := fs.ReadFile(vpsThemeFS, embedPath); err == nil {
				return content, mime.TypeByExtension(filepath.Ext(embedPath)), true
			}
		}

		// 3. 回退到嵌入式官方前端，后台页面需要它提供 /admin 路由
		if content, err := fs.ReadFile(defaultThemeFS, embedPath); err == nil {
			return content, mime.TypeByExtension(filepath.Ext(embedPath)), true
		}

		return nil, "", false
	}

	isRemovedAdminPage := func(reqPath string) bool {
		switch {
		case reqPath == "/admin/settings/reverse_proxy",
			reqPath == "/admin/settings/cloudflared",
			reqPath == "/admin/settings/notification",
			reqPath == "/admin/notification":
			return true
		default:
			return false
		}
	}

	// 核心逻辑：渲染 Index.html
	serveIndex := func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		cfg := getConfig()

		if reqPath == "/admin/settings/theme" {
			c.Redirect(http.StatusFound, "/admin/theme_managed")
			return
		}

		if isRemovedAdminPage(reqPath) {
			c.Redirect(http.StatusFound, "/admin")
			return
		}

		currentTheme := cfg[config.ThemeKey].(string)
		shouldReplace := true

		// 特殊页面：强制使用 default 主题，且不进行内容替换
		if strings.HasPrefix(reqPath, "/admin") || strings.HasPrefix(reqPath, "/terminal") {
			currentTheme = DefaultTheme
			shouldReplace = false
		}
		if reqPath == "/admin" && !hasValidSession(c) {
			serveLoginPage(c, cfg[config.SitenameKey].(string))
			return
		}
		if strings.HasPrefix(reqPath, "/admin") && hasValidSession(c) {
			if reqPath != "/admin" {
				c.Redirect(http.StatusFound, "/admin")
				return
			}
			adminHTML, _, exists := getFileContent(VpsTheme, path.Join(DistDir, "admin.html"))
			if exists {
				c.Header("Cache-Control", "no-store")
				adminHTMLStr := string(adminHTML)
				favTimestamp := getFaviconTimestamp()
				adminHTMLStr = strings.ReplaceAll(adminHTMLStr, `href="/favicon.ico"`, `href="/favicon.ico?t=`+favTimestamp+`"`)
				adminHTMLStr = strings.ReplaceAll(adminHTMLStr, `href="favicon.ico"`, `href="/favicon.ico?t=`+favTimestamp+`"`)
				c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(adminHTMLStr))
				return
			}
		}

		// 获取 dist/index.html (相对于主题根目录)
		targetFile := path.Join(DistDir, IndexFile)
		content, _, exists := getFileContent(currentTheme, targetFile)

		if !exists {
			c.String(http.StatusNotFound, "Index file missing (checked %s/dist/index.html and default).", currentTheme)
			return
		}

		htmlStr := string(content)
		favTimestamp := getFaviconTimestamp()
		htmlStr = strings.ReplaceAll(htmlStr, `href="/favicon.ico"`, `href="/favicon.ico?t=`+favTimestamp+`"`)
		htmlStr = strings.ReplaceAll(htmlStr, `href="favicon.ico"`, `href="/favicon.ico?t=`+favTimestamp+`"`)
		if language, err := c.Cookie(LanguageCookieName); err == nil {
			htmlStr = replaceHTMLLanguage(htmlStr, language)
		}

		// 如果不替换，保留系统内置页面内容，仅同步 html lang。
		if !shouldReplace {
			c.Header("Cache-Control", "no-store")
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlStr))
			return
		}

		// 执行 HTML 内容替换
		replacer := strings.NewReplacer(
			"<title>Monitor Monitor</title>", "<title>"+cfg[config.SitenameKey].(string)+"</title>",
			"A simple server monitor tool.", "VPS Monitor",
		)

		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(replacer.Replace(htmlStr)))
	}

	// ================= 路由定义 =================

	// Disable the original PWA service worker. The customized build serves live
	// admin/theme assets and stale precache entries can leave the admin as a blank page.
	r.GET("/sw.js", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(`
self.addEventListener('install', (event) => {
  self.skipWaiting();
});
self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    await self.registration.unregister();
  })());
});
`))
	})

	r.GET("/registerSW.js", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(`
(async () => {
  try {
    if ('serviceWorker' in navigator) {
      const registrations = await navigator.serviceWorker.getRegistrations();
      await Promise.all(registrations.map((registration) => registration.unregister()));
    }
    if ('caches' in window) {
      const keys = await caches.keys();
      await Promise.all(keys.map((key) => caches.delete(key)));
    }
  } catch {}
})();
`))
	})

	// 1. Favicon 优先策略
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		// 优先：./data/favicon.ico
		localFavicon := filepath.Join(DataDir, FaviconFile)
		if _, err := os.Stat(localFavicon); err == nil {
			c.File(localFavicon)
			return
		}

		// 没有自定义 favicon 时不再回退到主题默认图标，避免显示内置头像。
		c.Status(http.StatusNotFound)
	})

	// 2. 静态资源路由 /themes/:id/*path
	// 允许访问 /themes/MyTheme/theme.json 和 /themes/MyTheme/dist/assets/a.js
	r.GET("/themes/:id/*path", func(c *gin.Context) {
		themeID := c.Param("id")
		// c.Param("path") 包含了开头的 /，getFileContent 会处理
		filePath := c.Param("path")

		content, mimeType, exists := getFileContent(themeID, filePath)
		if exists {
			normalizedFilePath := filepath.ToSlash(filePath)
			if strings.HasSuffix(normalizedFilePath, "images/ethan-avatar.png") ||
				strings.HasSuffix(normalizedFilePath, "vps-clean.js") ||
				strings.HasSuffix(normalizedFilePath, "vps-admin-clean.js") {
				c.Header("Cache-Control", "no-store")
			}
			c.Data(http.StatusOK, mimeType, content)
			return
		}
		c.Status(http.StatusNotFound)
	})

	// 3. SPA 路由 (noRoute)
	noRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusNotFound)
			return
		}
		//
		func() {
			tempKey := c.Query("temp_key")
			if tempKey == "" {
				return
			}

			tempKeyExpireTime, err := config.GetAs[int64]("tempory_share_token_expire_at", 0)
			if err != nil {
				return
			}
			allowTempKey, err := config.GetAs[string]("tempory_share_token", "")
			if err != nil {
				return
			}

			if allowTempKey == "" || tempKey != allowTempKey {
				return
			}
			now := time.Now().Unix()
			if tempKeyExpireTime < now {
				return
			}
			expireSeconds := int(tempKeyExpireTime - now)
			if expireSeconds > 0 {
				c.SetCookie(
					"temp_key",    // key
					tempKey,       // value
					expireSeconds, // maxAge（秒）
					"/",           // path
					"",            // domain
					false,         // secure
					false,         // httpOnly
				)
			}
		}()
		reqPath := c.Request.URL.Path
		cfg := getConfig()
		currentTheme := cfg[config.ThemeKey].(string)

		if isRemovedAdminPage(reqPath) {
			c.Redirect(http.StatusFound, "/admin")
			return
		}

		// SPA 静态资源回退
		distPath := path.Join(DistDir, reqPath)

		content, mimeType, exists := getFileContent(currentTheme, distPath)
		if exists {
			normalizedDistPath := filepath.ToSlash(distPath)
			if strings.HasSuffix(normalizedDistPath, "images/ethan-avatar.png") ||
				strings.HasSuffix(normalizedDistPath, "vps-clean.js") ||
				strings.HasSuffix(normalizedDistPath, "vps-admin-clean.js") ||
				strings.HasSuffix(normalizedDistPath, ".js") ||
				strings.HasSuffix(normalizedDistPath, ".css") {
				c.Header("Cache-Control", "no-store")
			}
			c.Data(http.StatusOK, mimeType, content)
			return
		}

		// 如果资源不存在，且路径包含扩展名 (如 .js, .css, .png)，则返回 404
		// 避免将 index.html 作为 js 文件返回导致 "Failed to fetch dynamically imported module"
		//ext := filepath.Ext(reqPath)
		//if ext != "" && ext != ".html" {
		//	c.Status(http.StatusNotFound)
		//	return
		//}

		// 路由 (如 /dashboard, /settings) -> 返回 index.html
		serveIndex(c)
	})
}
