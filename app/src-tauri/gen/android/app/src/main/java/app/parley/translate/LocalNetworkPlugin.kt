package app.parley.translate

import android.Manifest
import android.app.Activity
import android.os.Build
import app.tauri.annotation.Command
import app.tauri.annotation.Permission
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin

// Declaring the permission on the plugin gives Tauri's permission helpers the
// strings and alias the Rust command asks for.
@TauriPlugin(
    permissions = [
        Permission(strings = [Manifest.permission.ACCESS_LOCAL_NETWORK], alias = "localNetwork")
    ]
)
class LocalNetworkPlugin(activity: Activity) : Plugin(activity) {
  // ACCESS_LOCAL_NETWORK only exists and is enforced on Android 17 (API 37);
  // older releases report an unknown permission as denied and would block LAN saves.
  @Command
  fun requestLocalNetwork(invoke: Invoke) {
    if (Build.VERSION.SDK_INT < 37) {
      invoke.resolve(JSObject().put("localNetwork", "granted"))
      return
    }
    requestPermissionForAliases(arrayOf("localNetwork"), invoke, "checkPermissions")
  }
}
