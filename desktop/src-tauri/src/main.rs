#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::io::{BufRead, BufReader};
use std::process::{Command, Stdio};
use std::sync::mpsc;
use std::thread;

use tauri::{Manager, WebviewUrl, WebviewWindowBuilder, RunEvent};

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
fn extract_field(line: &str, key: &str) -> Option<String> {
    let pat = format!("\"{}\":", key);
    let start = line.find(&pat)? + pat.len();
    let rest = &line[start..];
    if let Some(rest) = rest.strip_prefix('"') {
        let end = rest.find('"')?;
        Some(rest[..end].to_string())
    } else {
        let end = rest.find(|c: char| c == ',' || c == '}' || c == ' ')?;
        Some(rest[..end].to_string())
    }
}

fn main() {
    let builder = tauri::Builder::default().setup(|app| {
        // 可移植路径：相对安装目录的资源目录（macOS 为 .app/Contents/Resources，
        // Windows 为安装目录本身），不硬编码本机绝对路径
        let resource_dir = app
            .path()
            .resource_dir()
            .expect("无法获取应用资源目录");
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

        let mut cmd = Command::new(&go_bin);
        cmd.arg("--desktop")
            .env("STATIC_DIR", &web_dist)
            .env("DATA_DIR", &data_dir)
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit());
        hide_child_console(&mut cmd);
        let mut child = cmd.spawn().expect("failed to spawn Go backend");

        // 读取 Go 进程 stdout，捕获 {"event":"ready","port":N,"token":"T"}
        let stdout = child.stdout.take().expect("no stdout on Go backend");
        let (tx, rx) = mpsc::channel::<(u16, String)>();
        thread::spawn(move || {
            let reader = BufReader::new(stdout);
            for line in reader.lines().flatten() {
                if line.contains("\"event\":\"ready\"") {
                    if let (Some(port), Some(token)) =
                        (extract_field(&line, "port"), extract_field(&line, "token"))
                    {
                        if let Ok(port) = port.parse::<u16>() {
                            let _ = tx.send((port, token));
                            break;
                        }
                    }
                }
            }
        });

        let (port, boot_token) = rx.recv().expect("Go backend did not emit ready event");
        let url = format!("http://127.0.0.1:{}/?boot={}", port, boot_token);
        let _win = WebviewWindowBuilder::new(
            app,
            "main",
            WebviewUrl::External(url::Url::parse(&url).unwrap()),
        )
        .title("Terminal Web")
        .inner_size(1280.0, 800.0)
        .decorations(false)
        .build()
        .expect("failed to create window");

        app.manage(Backend(std::sync::Mutex::new(child)));
        Ok(())
    });

    let app = builder
        .build(tauri::generate_context!())
        .expect("failed to build Tauri app");

    app.run(|app_handle, event| {
        if let RunEvent::ExitRequested { .. } = event {
            if let Some(backend) = app_handle.try_state::<Backend>() {
                if let Ok(mut child) = backend.0.lock() {
                    let _ = child.kill();
                }
            }
        }
    });
}
