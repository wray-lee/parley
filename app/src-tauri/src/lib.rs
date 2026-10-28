// The shell holds no app logic: the Session core runs in-page (Device Host).
#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_websocket::init())
        .plugin(tauri_plugin_tts::init())
        .plugin(tauri_plugin_secure_keystore::init())
        .run(tauri::generate_context!())
        .expect("error while running Parley");
}
