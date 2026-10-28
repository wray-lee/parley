// The shell holds no app logic: the Session core runs in-page (Device Host).

#[cfg(target_os = "android")]
use tauri::Manager;

// Android 17 blocks LAN traffic for targetSdk 37 until ACCESS_LOCAL_NETWORK is
// granted. The state is the Kotlin plugin handle that owns the permission.
#[cfg(target_os = "android")]
struct LocalNetwork(tauri::plugin::PluginHandle<tauri::Wry>);

#[cfg(target_os = "android")]
#[tauri::command]
async fn request_local_network_access(
    state: tauri::State<'_, LocalNetwork>,
) -> Result<String, String> {
    let response: serde_json::Value = state
        .0
        .run_mobile_plugin_async("requestLocalNetwork", ())
        .await
        .map_err(|error| error.to_string())?;
    Ok(response["localNetwork"]
        .as_str()
        .unwrap_or("denied")
        .to_owned())
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
async fn request_local_network_access() -> Result<String, String> {
    Ok("granted".to_owned())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_websocket::init())
        .plugin(tauri_plugin_tts::init())
        .plugin(tauri_plugin_secure_keystore::init())
        .plugin(
            tauri::plugin::Builder::<tauri::Wry>::new("parley-local-network")
                .setup(|app, api| {
                    #[cfg(target_os = "android")]
                    app.manage(LocalNetwork(api.register_android_plugin(
                        "app.parley.translate",
                        "LocalNetworkPlugin",
                    )?));
                    #[cfg(not(target_os = "android"))]
                    let _ = (app, api);
                    Ok(())
                })
                .build(),
        )
        .invoke_handler(tauri::generate_handler![request_local_network_access])
        .run(tauri::generate_context!())
        .expect("error while running Parley");
}
