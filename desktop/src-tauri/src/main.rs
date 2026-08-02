#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::io::{BufRead, BufReader};
use std::process::{Command, Stdio};
use std::sync::mpsc;
use std::thread;
use std::time::Duration;

use url::Url;

use tauri::{Listener, Manager, WebviewUrl, WebviewWindowBuilder};
use tauri::window::Color;

// sidecar 可执行文件相对资源目录的路径：Windows 需要 .exe 后缀
#[cfg(windows)]
const SIDECAR_REL_PATH: &str = "binaries/terminal-web-server.exe";
#[cfg(not(windows))]
const SIDECAR_REL_PATH: &str = "binaries/terminal-web-server";

// Windows 下拉起子进程默认会弹出控制台黑窗，用 CREATE_NO_WINDOW 抑制。
#[cfg(windows)]
fn hide_child_console(cmd: &mut Command) {
    use std::os::windows::process::CommandExt;
    const CREATE_NO_WINDOW: u32 = 0x0800_0000;
    cmd.creation_flags(CREATE_NO_WINDOW);
}
#[cfg(not(windows))]
fn hide_child_console(_cmd: &mut Command) {}

// 托管的 Go 后端子进程，应用退出时一并杀掉。
struct Backend(std::sync::Mutex<std::process::Child>);

// 从 Go 打印的 JSON 行中抽取字段（兼容字符串值 "x" 与数值 123）。
fn extract_field<'a>(line: &'a str, key: &str) -> Option<&'a str> {
    let pat = format!("\"{}\":", key);
    let start = line.find(&pat)? + pat.len();
    let rest = &line[start..];
    if let Some(rest) = rest.strip_prefix('"') {
        let end = rest.find('"')?;
        Some(&rest[..end])
    } else {
        let end = rest.find(|c: char| c == ',' || c == '}' || c == ' ')?;
        Some(&rest[..end])
    }
}

// 从内置后端拉取用户在设置里配置的主题（dark/light），用于让启动画面与主窗口
// 背景跟随主题，而不是写死深色。本地 HTTP，毫秒级；任何失败都兜底 dark。
fn fetch_theme(port: u16) -> String {
    use std::io::{Read, Write};
    use std::net::TcpStream;
    use std::time::Duration;

    let host = "127.0.0.1";
    let mut stream = match TcpStream::connect((host, port)) {
        Ok(s) => s,
        Err(_) => return "dark".to_string(),
    };
    stream
        .set_read_timeout(Some(Duration::from_millis(800)))
        .ok();
    stream
        .set_write_timeout(Some(Duration::from_millis(800)))
        .ok();
    let req = format!(
        "GET /api/settings/public HTTP/1.1\r\nHost: {host}:{port}\r\nAccept: */*\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n"
    );
    if stream.write_all(req.as_bytes()).is_err() {
        return "dark".to_string();
    }
    let mut buf = Vec::new();
    if stream.read_to_end(&mut buf).is_err() {
        return "dark".to_string();
    }
    let body = String::from_utf8_lossy(&buf);
    if let Some(t) = extract_field(&body, "theme") {
        let t = t.to_string();
        if t == "light" || t == "dark" {
            return t;
        }
    }
    "dark".to_string()
}

// 把任意字符串做 percent-encode，使其可安全放进 data: URL（#、< 等字符必须编码，
// 否则 # 会被当成 URL fragment 截断 HTML）。
fn pct_encode(s: &str) -> String {
    let mut o = String::with_capacity(s.len());
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                o.push(b as char)
            }
            _ => o.push_str(&format!("%{:02X}", b)),
        }
    }
    o
}

// 启动失败时弹出一个清晰的错误窗口，而不是静默崩溃（闪退）。
fn show_err(app: &tauri::App, msg: &str) {
    let escaped = msg.replace('&', "&amp;").replace('<', "&lt;").replace('>', "&gt;");
    let html = format!(
        "<!doctype html><html><head><meta charset='utf-8'>\
         <style>body{{font-family:-apple-system,system-ui,'PingFang SC',sans-serif;\
         background:#1e1e1e;color:#eee;padding:32px;line-height:1.7;max-width:640px;margin:auto}}\
         h2{{color:#ff6b6b}}pre{{white-space:pre-wrap;word-break:break-all;background:#000;\
         padding:14px;border-radius:8px;color:#ff9b9b;font-size:13px}}</style></head>\
         <body><h2>Terminal Web 启动失败</h2><pre>{}</pre></body></html>",
        escaped
    );
    // Tauri v2 的 WebviewUrl 没有 Html 变体，改用 data: URL 通过 External 加载
    let data_url = format!("data:text/html;charset=utf-8,{}", pct_encode(&html));
    let url = Url::parse(&data_url).expect("构造错误窗口 data URL 失败");
    let _ = WebviewWindowBuilder::new(app, "error", WebviewUrl::External(url))
        .title("Terminal Web - 错误")
        .inner_size(640.0, 420.0)
        .resizable(true)
        .build();
}

fn main() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .setup(|app| {
            // 可移植路径：相对安装目录的资源目录（macOS 为 .app/Contents/Resources，
            // Windows 为安装目录本身），不硬编码本机绝对路径
            let resource_dir = match app.path().resource_dir() {
                Ok(d) => d,
                _ => {
                    show_err(app, "无法获取应用资源目录，应用无法继续。");
                    return Ok(());
                }
            };
            let go_bin = std::env::var("TERMINAL_WEB_GO_BIN").unwrap_or_else(|_| {
                resource_dir
                    .join(SIDECAR_REL_PATH)
                    .to_string_lossy()
                    .into_owned()
            });
            let web_dist = std::env::var("TERMINAL_WEB_STATIC_DIR").unwrap_or_else(|_| {
                resource_dir.join("web-dist").to_string_lossy().into_owned()
            });
            // 数据目录：可用 DATA_DIR 环境变量覆盖；默认放用户应用支持目录
            // （/Applications 只读，不能把数据写在 .app 内）
            let data_dir = std::env::var("DATA_DIR").unwrap_or_else(|_| {
                app.path()
                    .app_data_dir()
                    .expect("无法获取应用数据目录")
                    .to_string_lossy()
                    .into_owned()
            });

            // macOS：下载的安装包内嵌的 sidecar 会被打上 quarantine 标记，
            // Gatekeeper 会拒绝执行未签名的子进程。启动前尽力去掉该标记，
            // 若没有权限（例如在 /Applications 且属主非当前用户）则忽略，
            // 稍后会通过错误窗口提示用户手动执行 xattr 命令。
            #[cfg(target_os = "macos")]
            {
                let _ = std::process::Command::new("xattr")
                    .args(["-d", "com.apple.quarantine", go_bin.as_str()])
                    .output();
            }

            let mut cmd = Command::new(&go_bin);
            cmd.arg("--desktop")
                .env("STATIC_DIR", &web_dist)
                .env("DATA_DIR", &data_dir)
                .stdout(Stdio::piped())
                .stderr(Stdio::inherit());
            hide_child_console(&mut cmd);
            let mut child = match cmd.spawn() {
                Ok(c) => c,
                _ => {
                    show_err(
                        app,
                        &format!(
                            "无法启动内置服务端（Go 后端）。\n路径：{}\n\n常见原因与解决办法：\n\
                             1) macOS 提示“无法验证开发者”：在终端执行\n\
                                xattr -dr com.apple.quarantine /Applications/TerminalWeb.app\n\
                              （把 /Applications/TerminalWeb.app 换成你的实际路径），然后重新打开。\n\
                             2) 安装包不完整或被拦截：请重新从 GitHub Releases 下载。",
                            go_bin
                        ),
                    );
                    return Ok(());
                }
            };

            // 读取 Go 进程 stdout，捕获 {"event":"ready","port":N,"token":"T"}
            let stdout = match child.stdout.take() {
                Some(s) => s,
                None => {
                    show_err(app, "内置服务端未提供标准输出，无法读取启动端口。");
                    return Ok(());
                }
            };
            let (tx, rx) = mpsc::channel::<(u16, String)>();
            thread::spawn(move || {
                let reader = BufReader::new(stdout);
                for line in reader.lines().flatten() {
                    if line.contains("\"event\":\"ready\"") {
                        if let (Some(port), Some(token)) =
                            (extract_field(&line, "port"), extract_field(&line, "token"))
                        {
                            if let Ok(port) = port.parse::<u16>() {
                                let _ = tx.send((port, token.to_string()));
                                break;
                            }
                        }
                    }
                }
            });

            // 等待后端就绪，最多 15 秒；超时说明子进程未启动成功（常被 Gatekeeper 拦截）。
            let (port, boot_token) = match rx.recv_timeout(Duration::from_secs(15)) {
                Ok(v) => v,
                _ => {
                    show_err(
                        app,
                        "内置服务端启动超时，未收到 ready 事件。\n\
                         若在 macOS 上运行，多半是 Gatekeeper 拦截了未签名的子进程。\n\
                         请在终端执行：\n  xattr -dr com.apple.quarantine /Applications/TerminalWeb.app\n\
                         然后重新打开应用。",
                    );
                    return Ok(());
                }
            };
            // 读取用户在设置里配置的主题（dark/light），让启动画面与主窗口背景跟随主题，
            // 而不是写死深色。后端 public 接口返回 theme，异常时兜底 dark。
            let theme = fetch_theme(port);
            let theme_bg = if theme == "light" {
                Color(245, 246, 248, 255)
            } else {
                Color(16, 16, 20, 255)
            };

            let url = format!(
                "http://127.0.0.1:{}/?boot={}&theme={}",
                port, boot_token, theme
            );
            let parsed = match url::Url::parse(&url) {
                Ok(u) => u,
                _ => {
                    show_err(app, &format!("内部错误：启动地址非法：{}", url));
                    return Ok(());
                }
            };

            // 1) 先弹原生 splashscreen（纯静态 HTML，零 JS，瞬间绘制）。先创建使其立即渲染，
            //    设主题背景避免自身 WKWebView 首帧闪白，并把 ?theme= 传给 splash 让它内部的
            //    spinner/文字也跟随主题（见 web/splashscreen.html）。
            let splash = match WebviewWindowBuilder::new(
                app,
                "splashscreen",
                WebviewUrl::App(format!("splashscreen.html?theme={}", theme).into()),
            )
            .title("Terminal Web")
            .inner_size(1280.0, 800.0)
            .decorations(false)
            .background_color(theme_bg)
            .build()
            {
                Ok(w) => w,
                Err(_) => {
                    show_err(
                        app,
                        "无法创建启动画面窗口，应用仍可继续使用。",
                    );
                    return Ok(());
                }
            };

            // 2) 再创建主窗口。主窗口创建后默认为最上层（macOS 最后创建的窗口在上），
            //    会盖住 splash。所以创建主窗口后，立即把 splash 提到最前面，确保用户
            //    只看得到 splash 而看不到主窗口的加载过程。
            if WebviewWindowBuilder::new(app, "main", WebviewUrl::External(parsed))
                .title("Terminal Web")
                .inner_size(1280.0, 800.0)
                .decorations(false)
                .background_color(theme_bg)
                .build()
                .is_err()
            {
                show_err(app, "无法创建主窗口，请检查系统是否禁用了应用创建窗口。");
                return Ok(());
            }
            // 把 splash 窗口提到最前面（盖住刚创建的主窗口的首帧白闪）
            let _ = splash.set_focus();

            // 3) 前端就绪后：先显示主窗口（此时它仍在 splashscreen 之下被盖住，即使 webview
            //     首帧未就绪也看不见），再关闭 splashscreen，确保撤掉 splash 时主窗口已是深色内容。
            let handle = app.handle().clone();
            let on_ready = handle.clone();
            let _ = handle.listen("app-ready", move |_event| {
                if let Some(main) = on_ready.get_webview_window("main") {
                    let _ = main.show();
                    let _ = main.set_focus();
                }
                if let Some(splash) = on_ready.get_webview_window("splashscreen") {
                    let _ = splash.close();
                }
            });

            // 4) 兜底：若前端因异常未发出 app-ready，6 秒后先显示主窗口再关闭 splash
            let fb = app.handle().clone();
            std::thread::spawn(move || {
                std::thread::sleep(std::time::Duration::from_secs(6));
                if let Some(main) = fb.get_webview_window("main") {
                    let _ = main.show();
                }
                if let Some(splash) = fb.get_webview_window("splashscreen") {
                    let _ = splash.close();
                }
            });

            app.manage(Backend(std::sync::Mutex::new(child)));
            Ok(())
        });

    let app = builder
        .build(tauri::generate_context!())
        .expect("failed to build Tauri app");

    app.run(|app_handle, event| {
        if let tauri::RunEvent::ExitRequested { .. } = event {
            if let Some(backend) = app_handle.try_state::<Backend>() {
                if let Ok(mut child) = backend.0.lock() {
                    let _ = child.kill();
                }
            }
        }
    });
}
